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

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	shardingpredicate "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/predicate"
)

// ControllerName is the name of this controller.
const ControllerName = "sharder"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Reader == nil {
		r.Reader = mgr.GetAPIReader()
	}
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}

	return builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		For(&shardingv1alpha1.ControllerRing{}, builder.WithPredicates(shardingpredicate.ControllerRingCreatedOrUpdated())).
		Watches(&coordinationv1.Lease{}, handler.EnqueueRequestsFromMapFunc(MapLeaseToControllerRing), builder.WithPredicates(r.LeasePredicate())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(r)
}

func MapLeaseToControllerRing(ctx context.Context, obj client.Object) []reconcile.Request {
	ring, ok := obj.GetLabels()[shardingv1alpha1.LabelControllerRing]
	if !ok {
		return nil
	}

	return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: ring}}}
}

func (r *Reconciler) LeasePredicate() predicate.Predicate {
	return predicate.And(
		shardingpredicate.IsShardLease(),
		shardingpredicate.ShardLeaseAvailabilityChanged(r.Clock),
	)
}
