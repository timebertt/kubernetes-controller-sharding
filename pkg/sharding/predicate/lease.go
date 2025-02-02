/*
Copyright 2025 Tim Ebert.

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

package predicate

import (
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
)

// IsShardLease filters for events on Lease objects that have a non-empty label specifying the ControllerRing.
func IsShardLease() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		lease, ok := obj.(*coordinationv1.Lease)
		if !ok {
			return false
		}

		return lease.Labels[shardingv1alpha1.LabelControllerRing] != ""
	})
}

// ShardLeaseStateChanged reacts on lease events where the shard's state changes.
func ShardLeaseStateChanged(clock clock.PassiveClock) predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldLease, ok := e.ObjectOld.(*coordinationv1.Lease)
			if !ok {
				return false
			}
			newLease, ok := e.ObjectNew.(*coordinationv1.Lease)
			if !ok {
				return false
			}

			// only react if the shard's state changed
			now := clock.Now()
			return leases.ToState(oldLease, now) != leases.ToState(newLease, now)
		},
	}
}

// ShardLeaseAvailabilityChanged reacts on lease events where the shard's availability changes.
func ShardLeaseAvailabilityChanged(clock clock.PassiveClock) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			lease, ok := e.Object.(*coordinationv1.Lease)
			if !ok {
				return false
			}

			// only react if the new shard is available right away
			return leases.ToState(lease, clock.Now()).IsAvailable()
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

			// only react if the shard's availability changed
			now := clock.Now()
			return leases.ToState(oldLease, now).IsAvailable() != leases.ToState(newLease, now).IsAvailable()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			lease, ok := e.Object.(*coordinationv1.Lease)
			if !ok {
				return false
			}

			if e.DeleteStateUnknown {
				// If we missed the delete event, we cannot know the final state of the shard (available or not) for sure.
				// The included object might be stale, as we might have missed a relevant update before as well.
				// In this case, react for safety.
				return true
			}

			// only react if the removed shard was still available
			return leases.ToState(lease, clock.Now()).IsAvailable()
		},
	}
}
