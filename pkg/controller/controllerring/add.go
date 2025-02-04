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

package controllerring

import (
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	shardinghandler "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/handler"
	shardingpredicate "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/predicate"
)

// ControllerName is the name of this controller.
const ControllerName = "controllerring"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = client.WithFieldOwner(mgr.GetClient(), ControllerName+"-controller")
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-controller")
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}

	return builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		For(&shardingv1alpha1.ControllerRing{}, builder.WithPredicates(shardingpredicate.ControllerRingCreatedOrUpdated())).
		Watches(
			&coordinationv1.Lease{},
			handler.EnqueueRequestsFromMapFunc(shardinghandler.MapLeaseToControllerRing),
			builder.WithPredicates(r.LeasePredicate()),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(r)
}

func (r *Reconciler) LeasePredicate() predicate.Predicate {
	return predicate.And(
		shardingpredicate.IsShardLease(),
		predicate.Or(
			// react if a shard lease was created or deleted independent of its availability, we want to update
			// ControllerRing.status.shards
			predicate.Funcs{
				CreateFunc: func(_ event.CreateEvent) bool { return true },
				UpdateFunc: func(_ event.UpdateEvent) bool { return false },
				DeleteFunc: func(_ event.DeleteEvent) bool { return true },
			},
			// for update events, only react if the shard's availability changed
			shardingpredicate.ShardLeaseAvailabilityChanged(r.Clock),
		),
	)
}
