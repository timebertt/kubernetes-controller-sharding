/*
Copyright 2022 Tim Ebert.

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

package generator

import (
	"context"
	"reflect"
	"time"

	"golang.org/x/time/rate"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const defaultReconcileWorkers = 10

// Every runs the given Func with the specified frequency.
type Every struct {
	client.Client

	Name    string
	Do      func(ctx context.Context, c client.Client) error
	Rate    rate.Limit
	Stop    time.Time
	Workers int
}

func (r *Every) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	initialDelay := time.Duration(0)
	workers := defaultReconcileWorkers
	if r.Workers > 0 {
		workers = r.Workers
	}

	var rateLimiter workqueue.TypedRateLimiter[reconcile.Request] = &workqueue.TypedBucketRateLimiter[reconcile.Request]{
		Limiter: rate.NewLimiter(r.Rate, int(r.Rate)),
	}
	if r.Rate < 1 {
		// Special case for controllers running less frequent than every second:
		// The token bucket rate limiter would not allow any events as burst is less than 1, so replace it with a custom
		// rate limiter that always returns a constant delay.
		// Also, delay the first request when starting the scenario.
		every := time.Duration(1 / float64(r.Rate) * float64(time.Second))
		rateLimiter = constantDelayRateLimiter(every)
		initialDelay = every
		workers = 1
	}

	return builder.ControllerManagedBy(mgr).
		Named(r.Name).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: workers,
			RateLimiter:             rateLimiter,
		}).
		WatchesRawSource(EmitN(workers, initialDelay)).
		Complete(StopOnContextCanceled(r))
}

func (r *Every) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	if !r.Stop.IsZero() && !time.Now().Before(r.Stop) {
		// stop now
		return reconcile.Result{}, nil
	}

	return reconcile.Result{Requeue: true}, r.Do(ctx, r.Client)
}

// ForEach runs the given Func for each object of the given kind with the specified frequency.
// The first execution runs Every after object creation.
type ForEach[T client.Object] struct {
	client.Client

	Name    string
	Do      func(ctx context.Context, c client.Client, obj T) error
	Every   time.Duration
	Stop    time.Time
	Workers int

	gvk schema.GroupVersionKind
	obj T
}

func (r *ForEach[T]) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	var t T
	r.obj = reflect.New(reflect.TypeOf(t).Elem()).Interface().(T)

	var err error
	r.gvk, err = apiutil.GVKForObject(r.obj, mgr.GetScheme())
	if err != nil {
		return err
	}

	workers := defaultReconcileWorkers
	if r.Workers > 0 {
		workers = r.Workers
	}

	return builder.ControllerManagedBy(mgr).
		Named(r.Name).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: workers,
			RateLimiter:             unlimitedRateLimiter(),
		}).
		Watches(
			r.obj,
			&handler.Funcs{
				// only enqueue create events, after that we reconcile periodically
				CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
					if e.Object == nil {
						return
					}

					// we should not enqueue directly on creation, but only after Every has passed for the first time
					q.AddAfter(reconcile.Request{NamespacedName: types.NamespacedName{
						Name:      e.Object.GetName(),
						Namespace: e.Object.GetNamespace(),
					}}, r.Every)
				},
			},
		).
		Complete(StopOnContextCanceled(r))
}

func (r *ForEach[T]) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	if !r.Stop.IsZero() && !time.Now().Before(r.Stop) {
		// stop now
		return reconcile.Result{}, nil
	}

	obj := r.obj.DeepCopyObject().(T)
	if err := r.Get(ctx, request.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: r.Every}, r.Do(ctx, r.Client, obj)
}

// unlimitedRateLimiter returns a RateLimiter that doesn't apply any rate limits to the workqueue.
func unlimitedRateLimiter() workqueue.TypedRateLimiter[reconcile.Request] {
	// if no limiter is given, MaxOfRateLimiter returns 0 for When and NumRequeues => unlimited
	return &workqueue.TypedMaxOfRateLimiter[reconcile.Request]{}
}
