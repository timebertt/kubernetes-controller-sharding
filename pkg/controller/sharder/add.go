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
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	shardinghandler "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/handler"
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
		Watches(
			&coordinationv1.Lease{},
			handler.EnqueueRequestsFromMapFunc(shardinghandler.MapLeaseToControllerRing),
			builder.WithPredicates(r.LeasePredicate()),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
			// This custom rate limiter differs from the default in these aspects:
			// - no overall rate limiting, only per-item rate limiting
			// - start at 5s base delay, faster retries are probably wasted load on the API server
			// - cap delay at the configured sync period
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](5*time.Second, r.Config.Controller.Sharder.SyncPeriod.Duration),
		}).
		Complete(r)
}

func (r *Reconciler) LeasePredicate() predicate.Predicate {
	return predicate.And(
		shardingpredicate.IsShardLease(),
		shardingpredicate.ShardLeaseAvailabilityChanged(r.Clock),
	)
}
