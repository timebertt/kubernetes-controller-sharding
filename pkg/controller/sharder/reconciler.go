/*
Copyright 2023 Tim Ebert.

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

package sharder

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/consistenthash"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	shardingmetrics "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/metrics"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/ring"
	utilclient "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/client"
	utilerrors "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/errors"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/pager"
)

//+kubebuilder:rbac:groups=sharding.timebertt.dev,resources=clusterrings,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Note: The sharder requires permissions to list and patch resources listed in ClusterRings. However, the default
// sharder role doesn't include permissions for listing/mutating arbitrary resources (which would basically be
// cluster-admin access) to adhere to the least privilege principle.
// We can't automate permission management in the clusterring controller, because you can't grant permissions you don't
// already have.
// Hence, users need to grant the sharder permissions for listing/mutating sharded resources explicitly.

// Reconciler reconciles ClusterRings.
type Reconciler struct {
	Client client.Client
	Reader client.Reader
	Clock  clock.PassiveClock
	Config *configv1alpha1.SharderConfig
}

// Reconcile reconciles a ClusterRing object.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	clusterRing := &shardingv1alpha1.ClusterRing{}
	if err := r.Client.Get(ctx, req.NamespacedName, clusterRing); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	log = log.WithValues("ring", client.ObjectKeyFromObject(clusterRing))

	// collect list of shards in the ring
	leaseList := &coordinationv1.LeaseList{}
	if err := r.Client.List(ctx, leaseList, client.MatchingLabelsSelector{Selector: clusterRing.LeaseSelector()}); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing Leases for ClusterRing: %w", err)
	}

	// get ring and shards from cache
	hashRing, shards := ring.FromLeases(clusterRing, leaseList, r.Clock.Now())

	namespaces, err := r.getSelectedNamespaces(ctx, clusterRing)
	if err != nil {
		return reconcile.Result{}, err
	}

	// resync all resources in the ring
	if err := r.resyncClusterRing(ctx, log, clusterRing, namespaces, hashRing, shards); err != nil {
		return reconcile.Result{}, err
	}

	// requeue for periodic resync
	return reconcile.Result{RequeueAfter: r.Config.Controller.Sharder.SyncPeriod.Duration}, nil
}

func (r *Reconciler) getSelectedNamespaces(ctx context.Context, clusterRing *shardingv1alpha1.ClusterRing) (sets.Set[string], error) {
	namespaceSelector := r.Config.Webhook.Config.NamespaceSelector
	if clusterRing.Spec.NamespaceSelector != nil {
		namespaceSelector = clusterRing.Spec.NamespaceSelector
	}

	selector, err := metav1.LabelSelectorAsSelector(namespaceSelector)
	if err != nil {
		return nil, reconcile.TerminalError(err)
	}

	namespaceList := &corev1.NamespaceList{}
	if err := r.Client.List(ctx, namespaceList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("error listing selected namespaces for ClusterRing: %w", err)
	}

	namespaceSet := sets.New[string]()
	for _, namespace := range namespaceList.Items {
		namespaceSet.Insert(namespace.Name)
	}

	return namespaceSet, err
}

type objectToMove struct {
	drain           bool
	gr              metav1.GroupResource
	gvk             schema.GroupVersionKind
	key             client.ObjectKey
	resourceVersion string
	currentShard    string
}

func (r *Reconciler) resyncClusterRing(
	ctx context.Context,
	log logr.Logger,
	clusterRing *shardingv1alpha1.ClusterRing,
	namespaces sets.Set[string],
	hashRing *consistenthash.Ring,
	shards leases.Shards,
) error {
	const concurrency = 100
	var (
		wg   sync.WaitGroup
		errs = make(chan error)
		work = make(chan objectToMove, concurrency)
	)

	// find all objects that need to be moved or drained, add them to work queue
	wg.Add(1)
	go func() {
		for _, ringResource := range clusterRing.Spec.Resources {
			errs <- r.resyncResource(ctx, ringResource.GroupResource, clusterRing, namespaces, hashRing, shards, false, work)

			for _, controlledResource := range ringResource.ControlledResources {
				errs <- r.resyncResource(ctx, controlledResource, clusterRing, namespaces, hashRing, shards, true, work)
			}
		}

		close(work)
		wg.Done()
	}()

	// read from work queue and perform drains and movements
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for processNextWorkItem(ctx, log, r.Client, work, errs, clusterRing) {
			}
		}()
	}

	// wait for all processors and collect all errors
	go func() {
		wg.Wait()
		close(errs)
	}()

	allErrs := &multierror.Error{
		ErrorFormat: utilerrors.FormatErrors,
	}
	for err := range errs {
		if err != nil {
			allErrs = multierror.Append(allErrs, err)
		}
	}

	// return a combined error if any occurred
	return allErrs.ErrorOrNil()
}

func (r *Reconciler) resyncResource(
	ctx context.Context,
	gr metav1.GroupResource,
	ring sharding.Ring,
	namespaces sets.Set[string],
	hashRing *consistenthash.Ring,
	shards leases.Shards,
	controlled bool,
	work chan<- objectToMove,
) error {
	gvks, err := r.Client.RESTMapper().KindsFor(schema.GroupVersionResource{Group: gr.Group, Resource: gr.Resource})
	if err != nil {
		return fmt.Errorf("error determining kinds for resource %q: %w", gr.String(), err)
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no kinds found for resource %q", gr.String())
	}

	var allErrs *multierror.Error

	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(gvks[0])
	err = pager.New(r.Reader).EachListItem(ctx, list,
		func(obj client.Object) error {
			if !namespaces.Has(obj.GetNamespace()) {
				return nil
			}

			allErrs = multierror.Append(allErrs, r.resyncObject(gr, obj.(*metav1.PartialObjectMetadata), ring, hashRing, shards, controlled, work))
			return nil
		},
		// List a recent version from the API server's watch cache by setting resourceVersion=0. This reduces the load on etcd
		// for ring resyncs. Listing from etcd with quorum read would be a scalability limitation/bottleneck.
		// If we try to move or drain an object with an old resourceVersion (conflict error), we will retry with exponential
		// backoff.
		// This trades retries for smaller impact of periodic resyncs (that don't require any action).
		utilclient.ResourceVersion("0"),
	)
	if err != nil {
		allErrs = multierror.Append(allErrs, fmt.Errorf("error listing %s: %w", gr.String(), err))
	}

	return allErrs.ErrorOrNil()
}

func (r *Reconciler) resyncObject(
	gr metav1.GroupResource,
	obj *metav1.PartialObjectMetadata,
	ring sharding.Ring,
	hashRing *consistenthash.Ring,
	shards leases.Shards,
	controlled bool,
	work chan<- objectToMove,
) error {
	keyFunc := sharding.KeyForObject
	if controlled {
		keyFunc = sharding.KeyForController
	}

	key, err := keyFunc(obj)
	if err != nil {
		return err
	}
	if key == "" {
		// object should not be assigned
		return nil
	}

	var (
		desiredShard = hashRing.Hash(key)
		currentShard = obj.Labels[ring.LabelShard()]
	)

	if desiredShard == "" {
		// if no shard is available, there's nothing we can do
		return nil
	}

	if desiredShard == currentShard {
		// object is correctly assigned, nothing to do here
		return nil
	}

	workItem := objectToMove{
		gr:              gr,
		gvk:             obj.GroupVersionKind(),
		key:             client.ObjectKeyFromObject(obj),
		resourceVersion: obj.ResourceVersion,
		currentShard:    currentShard,
	}

	if currentShard != "" && currentShard != desiredShard && shards.ByID(currentShard).State.IsAvailable() && !controlled {
		// If the object should be moved and the current shard is still available, we need to drain it.
		// We only drain non-controlled objects, the controller's main object is used as a synchronization point for
		// preventing concurrent reconciliations.
		workItem.drain = true
	}

	// At this point, the object is either unassigned or the current shard is not available.
	// We send a (potentially empty) patch to trigger an assignment by the sharder webhook.
	work <- workItem
	return nil
}

func processNextWorkItem(
	ctx context.Context,
	log logr.Logger,
	c client.Writer,
	work <-chan objectToMove,
	errs chan<- error,
	ring sharding.Ring,
) bool {
	select {
	case <-ctx.Done():
		return false
	case w, ok := <-work:
		if !ok {
			return false
		}

		obj := &metav1.PartialObjectMetadata{}
		obj.SetGroupVersionKind(w.gvk)
		obj.SetName(w.key.Name)
		obj.SetNamespace(w.key.Namespace)
		obj.SetResourceVersion(w.resourceVersion)

		log = log.WithValues("resource", w.gr, "object", w.key)
		if w.drain {
			log.V(1).Info("Draining object from shard", "currentShard", w.currentShard)
			errs <- drainObject(ctx, c, obj, w.gr, ring)
		} else {
			log.V(1).Info("Moving object")
			errs <- moveObject(ctx, c, obj, w.gr, ring)
		}
	}

	return true
}

func drainObject(
	ctx context.Context,
	c client.Writer,
	obj *metav1.PartialObjectMetadata,
	gr metav1.GroupResource,
	ring sharding.Ring,
) error {
	patch := fmt.Sprintf(
		// - use optimistic locking by including the object's current resourceVersion
		// - add drain label; object will go through the sharder webhook when shard removes the drain label, which will
		//   perform the assignment
		`{"metadata":{"resourceVersion":"%s","labels":{"%s":"true"}}}`,
		obj.ResourceVersion, ring.LabelDrain(),
	)

	if err := c.Patch(ctx, obj, client.RawPatch(types.MergePatchType, []byte(patch))); err != nil {
		return fmt.Errorf("error draining %s %q: %w", gr.String(), client.ObjectKeyFromObject(obj), err)
	}

	shardingmetrics.MovementsTotal.WithLabelValues(
		shardingv1alpha1.KindClusterRing, ring.GetNamespace(), ring.GetName(),
		gr.Group, gr.Resource,
	).Inc()

	return nil
}

func moveObject(
	ctx context.Context,
	c client.Writer,
	obj *metav1.PartialObjectMetadata,
	gr metav1.GroupResource,
	ring sharding.Ring,
) error {
	patch := fmt.Sprintf(
		// - use optimistic locking by including the object's current resourceVersion
		// - remove shard label
		// - remove drain label if it is still present, this might happen when trying to drain an object from a shard that
		//   just got unavailable
		`{"metadata":{"resourceVersion":"%s","labels":{"%s":null,"%s":null}}}`,
		obj.ResourceVersion, ring.LabelShard(), ring.LabelDrain(),
	)

	if err := c.Patch(ctx, obj, client.RawPatch(types.MergePatchType, []byte(patch))); err != nil {
		return fmt.Errorf("error triggering assignment for %s %q: %w", gr.String(), client.ObjectKeyFromObject(obj), err)
	}

	shardingmetrics.MovementsTotal.WithLabelValues(
		shardingv1alpha1.KindClusterRing, ring.GetNamespace(), ring.GetName(),
		gr.Group, gr.Resource,
	).Inc()

	return nil
}
