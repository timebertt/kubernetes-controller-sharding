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

package shardlease

import (
	"context"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	shardingpredicate "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/predicate"
)

// ControllerName is the name of this controller.
const ControllerName = "shardlease"

var handlerLog = logf.Log.WithName("controller").WithName(ControllerName)

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}

	return builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		For(&coordinationv1.Lease{}, builder.WithPredicates(r.LeasePredicate())).
		// enqueue all Leases belonging to a ControllerRing when it is created or the spec is updated
		Watches(&shardingv1alpha1.ControllerRing{}, handler.EnqueueRequestsFromMapFunc(r.MapControllerRingToLeases), builder.WithPredicates(shardingpredicate.ControllerRingCreatedOrUpdated())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(r)
}

func (r *Reconciler) LeasePredicate() predicate.Predicate {
	return predicate.And(
		shardingpredicate.IsShardLease(),
		shardingpredicate.ShardLeaseStateChanged(r.Clock),
		// ignore deletion of shard leases
		predicate.Funcs{
			DeleteFunc: func(_ event.DeleteEvent) bool { return false },
		},
	)
}

func (r *Reconciler) MapControllerRingToLeases(ctx context.Context, obj client.Object) []reconcile.Request {
	controllerRing := obj.(*shardingv1alpha1.ControllerRing)

	leaseList := &coordinationv1.LeaseList{}
	if err := r.Client.List(ctx, leaseList, client.MatchingLabelsSelector{Selector: controllerRing.LeaseSelector()}); err != nil {
		handlerLog.Error(err, "failed listing Leases for ControllerRing", "controllerRing", client.ObjectKeyFromObject(controllerRing))
		return nil
	}

	requests := make([]reconcile.Request, 0, len(leaseList.Items))
	for _, l := range leaseList.Items {
		lease := l
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&lease)})
	}

	return requests
}
