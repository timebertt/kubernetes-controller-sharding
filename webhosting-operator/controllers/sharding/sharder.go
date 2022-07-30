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

package sharding

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/sharding"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/sharding/leases"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/consistenthash"
)

// shardingReconciler reconciles sharded objects and assigns them to shards.
type shardingReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
	Clock  clock.Clock
	logger logr.Logger

	Object         client.Object
	KeyForObject   KeyFunc
	LeaseNamespace string
	TokensPerNode  int

	groupKind  schema.GroupKind
	metaObject *metav1.PartialObjectMetadata
	metaList   *metav1.PartialObjectMetadataList
}

//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch

// Reconcile reconciles a sharded object.
func (r *shardingReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	obj := r.metaObject.DeepCopy()
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	leaseList := &coordinationv1.LeaseList{}
	// TODO: add labels to shard leases
	if err := r.Client.List(ctx, leaseList, client.InNamespace(r.LeaseNamespace)); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing shard leases: %w", err)
	}

	var (
		// determine ready shards and prepare hash ring
		shards      = leases.ToShards(leaseList.Items, r.Clock)
		readyShards = shards.ByState(leases.Uncertain).IDs()
		ring        = consistenthash.New(consistenthash.DefaultHash, r.TokensPerNode, readyShards...)

		// determine desired shard, might be empty if there is no ready shard
		desiredShard = ring.Hash(r.KeyForObject(r.groupKind, obj))
		currentShard = obj.Labels[sharding.ShardLabel]

		// requeue after some time to check if we need to rebalance
		requeueAfter = shards.ById(desiredShard).Times.ToExpired
	)

	log = log.WithValues("shard", desiredShard)

	if currentShard != "" && currentShard != desiredShard && stateNeedsDrain(shards.ById(currentShard).State) {
		log.Info("Draining object from shard", "currentShard", currentShard)

		patch := client.MergeFromWithOptions(obj.DeepCopy(), client.MergeFromWithOptimisticLock{})
		metav1.SetMetaDataLabel(&obj.ObjectMeta, sharding.DrainLabel, "true")
		if err := r.Client.Patch(ctx, obj, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("error draining object: %w", err)
		}

		// object will be requeued when drain was successful, shard released its lease or sharder acquired shard lease
		return reconcile.Result{}, nil
	}

	if desiredShard != currentShard {
		// at this point, the object is either unassigned or the current shard is dead
		log.Info("Assigning object to shard")

		patch := client.MergeFromWithOptions(obj.DeepCopy(), client.MergeFromWithOptimisticLock{})
		metav1.SetMetaDataLabel(&obj.ObjectMeta, sharding.ShardLabel, desiredShard)
		if err := r.Client.Patch(ctx, obj, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("error assigning object: %w", err)
		}
	}

	// requeue if we left object unassigned
	return reconcile.Result{Requeue: desiredShard == "", RequeueAfter: requeueAfter}, nil
}

func stateNeedsDrain(state leases.ShardState) bool {
	// drain as long as lease is not released or acquired by sharder
	return state > leases.Dead
}

const objectShardField = "metadata.labels.shard"

// SetupWithManager sets up the controller with the Manager.
func (r *shardingReconciler) SetupWithManager(mgr manager.Manager) error {
	if r.KeyForObject == nil {
		r.KeyForObject = DefaultKeyFunc
	}

	gvk, err := apiutil.GVKForObject(r.Object, r.Scheme)
	if err != nil {
		return fmt.Errorf("unable to determine GVK of %T for a metadata-only watch: %w", r.Object, err)
	}

	// prepare metadata-only object and list
	r.groupKind = gvk.GroupKind()
	r.metaObject = &metav1.PartialObjectMetadata{}
	r.metaObject.SetGroupVersionKind(gvk)

	listGVK := gvk
	listGVK.Kind += "List"
	r.metaList = &metav1.PartialObjectMetadataList{}
	r.metaList.SetGroupVersionKind(listGVK)

	// add index for fast mapping from lease to sharded objects
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), r.metaObject, objectShardField, func(obj client.Object) []string {
		return []string{obj.GetLabels()[sharding.ShardLabel]}
	}); err != nil {
		return err
	}

	c, err := builder.ControllerManagedBy(mgr).
		Named("sharder").
		For(r.metaObject, builder.WithPredicates(objectPredicate)).
		// watch leases to enqueue sharded objects on state changes
		Watches(
			&source.Kind{Type: &coordinationv1.Lease{}},
			handler.EnqueueRequestsFromMapFunc(r.MapLeaseToObjects),
			builder.WithPredicates(r.leasePredicate()),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RecoverPanic:            true,
		}).
		Build(r)
	if err != nil {
		return err
	}

	r.logger = c.GetLogger()

	return nil
}

// objectPredicate filters events to only enqueue sharded objects if they are unassigned
var objectPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		// always act on ADD events to reconcile assignments on startup
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetLabels()[sharding.ShardLabel] == ""
	},
	DeleteFunc: func(_ event.DeleteEvent) bool { return false },
}

// leasePredicate filters lease events to react on events that might need rebalancing.
func (r *shardingReconciler) leasePredicate() predicate.Predicate {
	return predicate.And(
		isShardLease(r.LeaseNamespace),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldLease, ok := e.ObjectOld.(*coordinationv1.Lease)
				if !ok {
					return false
				}
				newLease, ok := e.ObjectNew.(*coordinationv1.Lease)
				if !ok {
					return false
				}

				return leases.ToState(oldLease, r.Clock) != leases.ToState(newLease, r.Clock)
			},
			DeleteFunc: func(_ event.DeleteEvent) bool { return true },
		},
	)
}

// MapLeaseToObjects maps a lease to all sharded objects that are assigned to the corresponding shard
func (r *shardingReconciler) MapLeaseToObjects(lease client.Object) []reconcile.Request {
	objectList := r.metaList.DeepCopy()
	if err := r.Client.List(context.TODO(), objectList, client.MatchingFields{objectShardField: lease.GetName()}); err != nil {
		r.logger.Error(err, "failed to list sharded objects belonging to lease", "lease", client.ObjectKeyFromObject(lease))
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(objectList.Items))
	for i, website := range objectList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      website.GetName(),
				Namespace: website.GetNamespace(),
			},
		}
	}
	return requests
}
