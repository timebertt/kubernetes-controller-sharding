/*
Copyright 2024 Tim Ebert.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tracker

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

var (
	// NOTE: upper bound 5 is the SLO. Align buckets with SLO so that when p99 estimation grows above SLO we are sure
	// that p99 is actually above SLO.
	// See https://prometheus.io/docs/practices/histograms/#errors-of-quantile-estimation
	buckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

	websiteReconciliationLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "experiment",
		Subsystem: "website",
		Name:      "reconciliation_duration_seconds",
		Help: "Latency from Website creation or spec update until the generation is observed as ready by a watch. " +
			"This is an SLI for controller performance.",
		Buckets: buckets,
	}, nil)
	websiteBackfilledGenerations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "experiment",
		Subsystem: "website",
		Name:      "backfilled_generations_total",
		Help: "Counter for Website generations that were never observed as ready before the next generation appeared. " +
			"For such generations, the time when the next generation got ready is used for calculating the reconciliation latency. " +
			"This metric serves as an indicator for how accurate the reconciliation latency measurement is.",
	}, nil)
	watchLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "experiment",
		Name:      "watch_latency_seconds",
		Help: "Latency from Website transition time until observed by a watch event in experiment. " +
			"This metric serves as an indicator for how experiment itself is performing to ensure accurate measurements.",
		Buckets: buckets,
	}, nil)
)

// WebsiteTracker tracks how long it takes for generations of Website objects to be reconciled and get ready.
type WebsiteTracker struct {
	reader client.Reader

	// objects maps an object's UID to the object's generation information.
	// The UID is used as the key instead of a namespaced name so that the tracker can distinguish between object
	// instances with the same namespaced name.
	objects *tracker[types.UID, objectGenerations]
}

// objectGenerations stores information about all generations of a single object.
type objectGenerations struct {
	lock        sync.Mutex
	generations *tracker[int64, generationTimes]
}

// generationTimes stores information about a single generation of an object.
// It doesn't synchronize access. Access is synchronized by the lock on objectGenerations level.
type generationTimes struct {
	created, ready time.Time
	recorded       bool
}

func newObjectsTracker() *tracker[types.UID, objectGenerations] {
	return &tracker[types.UID, objectGenerations]{
		values: map[types.UID]*objectGenerations{},
		new: func() *objectGenerations {
			return &objectGenerations{
				generations: newGenerationsTracker(),
			}
		},
	}
}

func newGenerationsTracker() *tracker[int64, generationTimes] {
	return &tracker[int64, generationTimes]{
		values: map[int64]*generationTimes{},
	}
}

// AddToManager initializes the tracker, registers its metrics, and starts watching objects.
func (w *WebsiteTracker) AddToManager(mgr manager.Manager) error {
	if w.reader == nil {
		w.reader = mgr.GetCache()
	}

	w.objects = newObjectsTracker()

	// initialize counters
	websiteBackfilledGenerations.WithLabelValues().Add(0)

	// register all metrics
	metrics.Registry.MustRegister(websiteReconciliationLatency, websiteBackfilledGenerations, watchLatency)

	return builder.ControllerManagedBy(mgr).
		Named("website-tracker").
		Watches(&webhostingv1alpha1.Website{}, w.websiteHandler(), builder.WithPredicates(w.observedNewReadyGeneration())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(w)
}

// websiteHandler enqueues the object's UID for update requests.
// It concurrently adds the observed status to the tracker and enqueues the object for reconciliation with a short delay
// to give the tracker enough time to store the observed status.
// This reduces the number of requeues we need to do for waiting for the ready timestamp to arrive in the tracker.
func (w *WebsiteTracker) websiteHandler() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(_ context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			website, ok := e.ObjectNew.(*webhostingv1alpha1.Website)
			if !ok {
				return
			}

			w.recordStatusGeneration(website)

			q.AddAfter(reconcile.Request{NamespacedName: types.NamespacedName{
				Name: string(website.UID),
			}}, time.Second)
		},
	}
}

// observedNewReadyGeneration returns a predicate that triggers when a new generation is observed to be ready and its
// reconciliation latency should be recorded by the reconciler.
func (w *WebsiteTracker) observedNewReadyGeneration() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldWebsite, ok := e.ObjectOld.(*webhostingv1alpha1.Website)
			if !ok {
				return false
			}
			website, ok := e.ObjectNew.(*webhostingv1alpha1.Website)
			if !ok {
				return false
			}

			// Record how long it took experiment to observe the transition.
			if t := website.Status.LastTransitionTime; t != nil && !t.Equal(oldWebsite.Status.LastTransitionTime) {
				watchLatency.WithLabelValues().Observe(time.Since(t.Time).Seconds())
			}

			// if the status changed and the website is ready, we observed a new generation becoming ready
			return !apiequality.Semantic.DeepEqual(oldWebsite.Status, website.Status) &&
				website.Status.Phase == webhostingv1alpha1.PhaseReady
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}

// RecordSpecChange records a new generation of the given website as created or updated by the experiment.
// Supposed to be called by the load generator.
func (w *WebsiteTracker) RecordSpecChange(website *webhostingv1alpha1.Website) {
	// record time first, before locking
	t := time.Now()

	// store time concurrently, so that we don't block the load generator
	go w.objects.set(website.UID, func(generations *objectGenerations) {
		generations.setGenerationCreated(website.Generation, t)
	})
}

// recordStatusGeneration records a new observed generation of the given website if it is ready.
// Supposed to be called by the watch handler (synchronously).
func (w *WebsiteTracker) recordStatusGeneration(website *webhostingv1alpha1.Website) {
	// record time first, before locking
	t := time.Now()

	// store time concurrently, so that we don't block the watch handler/reflector
	go w.objects.set(website.UID, func(generations *objectGenerations) {
		generations.setGenerationReady(website.Status.ObservedGeneration, t)
	})
}

// setGenerationCreated records the time when the given generation was created, i.e., when the creation / spec change
// happened.
func (o *objectGenerations) setGenerationCreated(generation int64, t time.Time) {
	o.lock.Lock()
	defer o.lock.Unlock()

	o.generations.set(generation, func(times *generationTimes) {
		if !times.created.IsZero() {
			// generation was already recorded
			return
		}

		times.created = t
	})
}

// setGenerationReady records the time when the given generation was observed as ready.
func (o *objectGenerations) setGenerationReady(generation int64, t time.Time) {
	o.lock.Lock()
	defer o.lock.Unlock()

	o.generations.set(generation, func(times *generationTimes) {
		if !times.ready.IsZero() {
			// generation was already recorded
			return
		}

		times.ready = t
	})
}

// Reconcile iterates over all objects and records metrics for the generations that were observed to be ready.
func (w *WebsiteTracker) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)
	uid := types.UID(req.Name)

	var res reconcile.Result
	w.objects.set(uid, func(generations *objectGenerations) {
		res = generations.reconcile(log)
	})
	return res, nil
}

// reconcile iterates over the object's generations and records newly observed finished reconciliations in the latency
// metric.
func (o *objectGenerations) reconcile(log logr.Logger) reconcile.Result {
	o.lock.Lock()
	defer o.lock.Unlock()

	generations := generationsList(o.generations.sortedValues())
	generations.backfillGenerations()

	if finished := generations.recordNewReadyGenerations(log); !finished {
		return reconcile.Result{Requeue: true}
	}

	return reconcile.Result{}
}

type generationsList []*generationTimes

// backfillGenerations backfills the ready time for generations if we observed a newer ready generation.
// This might be necessary if we missed some watch events, or multiple generations were created in quick succession
// where only a later one was reconciled by the controller to be ready.
// The generations slice must be sorted by generation in ascending order.
func (gg generationsList) backfillGenerations() {
	var newerReadyTime time.Time
	for i := len(gg) - 1; i >= 0; i-- {
		g := gg[i]

		if g.recorded {
			continue
		}

		if g.ready.IsZero() {
			// this generation was not observed as ready,
			// copy the ready time from the generation after it (if it has been observed as ready)
			g.ready = newerReadyTime
			websiteBackfilledGenerations.WithLabelValues().Add(1)
		} else {
			// this generation was observed as ready, copy the timestamp for backfilling earlier generations.
			newerReadyTime = g.ready
		}
	}
}

// recordNewReadyGenerations records all newly observed ready generations.
// Returns true if all generations have been recorded, false if there are unready generations left.
func (gg generationsList) recordNewReadyGenerations(log logr.Logger) bool {
	finished := true
	for _, g := range gg {
		if g.recorded {
			continue
		}
		if g.created.IsZero() || g.ready.IsZero() {
			finished = false
			continue
		}

		latency := g.ready.Sub(g.created)
		logLevel := 1
		if latency > 30*time.Second {
			logLevel = 0
		}
		log.V(logLevel).Info("Observed ready website", "latency", latency, "created", g.created, "ready", g.ready)

		websiteReconciliationLatency.WithLabelValues().Observe(latency.Seconds())
		g.recorded = true
	}

	return finished
}
