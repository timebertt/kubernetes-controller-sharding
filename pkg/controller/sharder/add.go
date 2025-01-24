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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
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
		For(&shardingv1alpha1.ControllerRing{}, builder.WithPredicates(r.ControllerRingPredicate())).
		Watches(&coordinationv1.Lease{}, handler.EnqueueRequestsFromMapFunc(MapLeaseToControllerRing), builder.WithPredicates(r.LeasePredicate())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(r)
}

func (r *Reconciler) ControllerRingPredicate() predicate.Predicate {
	return predicate.And(
		predicate.GenerationChangedPredicate{},
		// ignore deletion of ControllerRings
		predicate.Funcs{
			CreateFunc: func(_ event.CreateEvent) bool { return true },
			UpdateFunc: func(_ event.UpdateEvent) bool { return true },
			DeleteFunc: func(_ event.DeleteEvent) bool { return false },
		},
	)
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
		predicate.NewPredicateFuncs(isShardLease),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				lease, ok := e.Object.(*coordinationv1.Lease)
				if !ok {
					return false
				}

				// We only need to resync the ring if the new shard is available right away.
				// Note: on controller start we will enqueue anyway for the add event of ControllerRings.
				return leases.ToState(lease, r.Clock.Now()).IsAvailable()
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldLease, ok := e.ObjectOld.(*coordinationv1.Lease)
				if !ok {
					return false
				}
				newLease, ok := e.ObjectNew.(*coordinationv1.Lease)
				if !ok {
					return false
				}

				// We only need to resync the ring if the shard's availability changed.
				now := r.Clock.Now()
				return leases.ToState(oldLease, now).IsAvailable() != leases.ToState(newLease, now).IsAvailable()
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				if e.DeleteStateUnknown {
					// If we missed the delete event, we cannot know the final state of the shard (available or not) for sure.
					// The included object might be stale, as we might have missed a relevant update before as well.
					// Leases usually go unavailable before being deleted, so its unlikely that we need to act on this delete
					// event.
					// Hence, skip enqueing here in favor of periodic resyncs.
					return true
				}

				lease, ok := e.Object.(*coordinationv1.Lease)
				if !ok {
					return false
				}

				// We only need to resync the ring if the removed shard was still available.
				return leases.ToState(lease, r.Clock.Now()).IsAvailable()
			},
		},
	)
}

func isShardLease(obj client.Object) bool {
	return obj.GetLabels()[shardingv1alpha1.LabelControllerRing] != ""
}
