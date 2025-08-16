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
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/consistenthash"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/key"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	shardingmetrics "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/metrics"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/ring"
	utilclient "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/client"
	utilerrors "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/errors"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/pager"
)

//+kubebuilder:rbac:groups=sharding.timebertt.dev,resources=controllerrings,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Note: The sharder requires permissions to list and patch resources listed in ControllerRings. However, the default
// sharder role doesn't include permissions for listing/mutating arbitrary resources (which would basically be
// cluster-admin access) to adhere to the least privilege principle.
// We can't automate permission management in the controllerring controller, because you can't grant permissions you don't
// already have.
// Hence, users need to grant the sharder permissions for listing/mutating sharded resources explicitly.

// Reconciler reconciles ControllerRings.
type Reconciler struct {
	Client client.Client
	Reader client.Reader
	Clock  clock.PassiveClock
	Config *configv1alpha1.SharderConfig
}

// Reconcile reconciles a ControllerRing object.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	controllerRing := &shardingv1alpha1.ControllerRing{}
	if err := r.Client.Get(ctx, req.NamespacedName, controllerRing); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	o, err := r.NewOperation(ctx, controllerRing)
	if err != nil {
		return reconcile.Result{}, err
	}

	log.Info("Starting resync of object assignments for ControllerRing")
	defer func(start time.Time) {
		log.V(1).Info("Finished resync of object assignments for ControllerRing", "duration", r.Clock.Since(start))
	}(r.Clock.Now())

	if err := o.ResyncControllerRing(ctx, log); err != nil {
		return reconcile.Result{}, err
	}

	// requeue for periodic resync
	return reconcile.Result{RequeueAfter: r.Config.Controller.Sharder.SyncPeriod.Duration}, nil
}

func (r *Reconciler) NewOperation(ctx context.Context, controllerRing *shardingv1alpha1.ControllerRing) (*Operation, error) {
	// collect list of shards in the ring
	leaseList := &coordinationv1.LeaseList{}
	if err := r.Client.List(ctx, leaseList, client.MatchingLabelsSelector{Selector: controllerRing.LeaseSelector()}); err != nil {
		return nil, fmt.Errorf("error listing Leases for ControllerRing: %w", err)
	}

	// get ring and shards from cache
	hashRing, shards := ring.FromLeases(controllerRing, leaseList, r.Clock.Now())

	namespaces, err := r.GetSelectedNamespaces(ctx, controllerRing)
	if err != nil {
		return nil, err
	}

	return &Operation{
		Client:         r.Client,
		Reader:         r.Reader,
		ControllerRing: controllerRing,
		Namespaces:     namespaces,
		HashRing:       hashRing,
		Shards:         shards,
	}, nil
}

func (r *Reconciler) GetSelectedNamespaces(ctx context.Context, controllerRing *shardingv1alpha1.ControllerRing) (sets.Set[string], error) {
	namespaceSelector := r.Config.Webhook.Config.NamespaceSelector
	if controllerRing.Spec.NamespaceSelector != nil {
		namespaceSelector = controllerRing.Spec.NamespaceSelector
	}

	selector, err := metav1.LabelSelectorAsSelector(namespaceSelector)
	if err != nil {
		return nil, reconcile.TerminalError(err)
	}

	namespaceList := &corev1.NamespaceList{}
	if err := r.Client.List(ctx, namespaceList, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, fmt.Errorf("error listing selected namespaces for ControllerRing: %w", err)
	}

	namespaceSet := sets.New[string]()
	for _, namespace := range namespaceList.Items {
		namespaceSet.Insert(namespace.Name)
	}

	return namespaceSet, err
}

type Operation struct {
	Client client.Client
	Reader client.Reader

	ControllerRing *shardingv1alpha1.ControllerRing
	Namespaces     sets.Set[string]
	HashRing       *consistenthash.Ring
	Shards         leases.Shards
}

func (o *Operation) ResyncControllerRing(ctx context.Context, log logr.Logger) error {
	allErrs := &multierror.Error{
		ErrorFormat: utilerrors.FormatErrors,
	}

	// resync all ring resources
	for _, ringResource := range o.ControllerRing.Spec.Resources {
		allErrs = multierror.Append(allErrs,
			o.resyncResource(ctx, log, ringResource.GroupResource, false),
		)

		for _, controlledResource := range ringResource.ControlledResources {
			allErrs = multierror.Append(allErrs,
				o.resyncResource(ctx, log, controlledResource, true),
			)
		}
	}

	// collect all errors and return a combined error if any occurred
	return allErrs.ErrorOrNil()
}

func (o *Operation) resyncResource(
	ctx context.Context,
	log logr.Logger,
	gr metav1.GroupResource,
	controlled bool,
) error {
	log = log.WithValues("resource", gr)

	gvks, err := o.Client.RESTMapper().KindsFor(schema.GroupVersionResource{Group: gr.Group, Resource: gr.Resource})
	if err != nil {
		return fmt.Errorf("error determining kinds for resource %q: %w", gr.String(), err)
	}
	if len(gvks) == 0 {
		return fmt.Errorf("no kinds found for resource %q", gr.String())
	}

	var allErrs *multierror.Error

	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(gvks[0])
	err = pager.New(o.Reader).EachListItemWithAlloc(ctx, list,
		func(obj client.Object) error {
			if !o.Namespaces.Has(obj.GetNamespace()) {
				return nil
			}

			allErrs = multierror.Append(allErrs, o.resyncObject(ctx, log, gr, controlled, obj.(*metav1.PartialObjectMetadata)))
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

var (
	// KeyForObject is an alias for key.ForObject, exposed for testing.
	KeyForObject = key.ForObject
	// KeyForController is an alias for key.ForController, exposed for testing.
	KeyForController = key.ForController
)

func (o *Operation) resyncObject(
	ctx context.Context,
	log logr.Logger,
	gr metav1.GroupResource,
	controlled bool,
	obj *metav1.PartialObjectMetadata,
) error {
	log = log.WithValues("object", client.ObjectKeyFromObject(obj))

	keyFunc := KeyForObject
	if controlled {
		keyFunc = KeyForController
	}

	hashKey, err := keyFunc(obj)
	if err != nil {
		return err
	}
	if hashKey == "" {
		// object should not be assigned
		return nil
	}

	var (
		desiredShard = o.HashRing.Hash(hashKey)
		currentShard = obj.Labels[o.ControllerRing.LabelShard()]
	)

	if desiredShard == "" {
		// if no shard is available, there's nothing we can do
		return nil
	}

	if desiredShard == currentShard {
		// object is correctly assigned, nothing to do here
		return nil
	}

	if currentShard != "" && o.Shards.ByID(currentShard).State.IsAvailable() && !controlled {
		// If the object should be moved and the current shard is still available, we need to drain it.
		// We only drain non-controlled objects, the controller's main object is used as a synchronization point for
		// preventing concurrent reconciliations.
		log.V(1).Info("Draining object from shard", "currentShard", currentShard)

		patch := client.MergeFromWithOptions(obj.DeepCopy(), client.MergeFromWithOptimisticLock{})
		metav1.SetMetaDataLabel(&obj.ObjectMeta, o.ControllerRing.LabelDrain(), "true")
		if err := o.Client.Patch(ctx, obj, patch); err != nil {
			return fmt.Errorf("error draining %s %q: %w", gr.String(), client.ObjectKeyFromObject(obj), err)
		}

		shardingmetrics.DrainsTotal.WithLabelValues(
			o.ControllerRing.Name, gr.Group, gr.Resource,
		).Inc()

		// object will go through the sharder webhook when shard removes the drain label, which will perform the assignment
		return nil
	}

	// At this point, the object is either unassigned or the current shard is not available.
	// We send a (potentially empty) patch to trigger an assignment by the sharder webhook.
	log.V(1).Info("Moving object")

	patch := client.MergeFromWithOptions(obj.DeepCopy(), client.MergeFromWithOptimisticLock{})
	// remove drain label if it is still present, this might happen when trying to drain an object from a shard that
	// just got unavailable
	delete(obj.Labels, o.ControllerRing.LabelShard())
	delete(obj.Labels, o.ControllerRing.LabelDrain())
	if err := o.Client.Patch(ctx, obj, patch); err != nil {
		return fmt.Errorf("error triggering assignment for %s %q: %w", gr.String(), client.ObjectKeyFromObject(obj), err)
	}

	shardingmetrics.MovementsTotal.WithLabelValues(
		o.ControllerRing.Name, gr.Group, gr.Resource,
	).Inc()

	return nil
}
