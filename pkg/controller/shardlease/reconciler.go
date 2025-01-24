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
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
)

//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;update;patch;delete

// Reconciler reconciles shard leases.
type Reconciler struct {
	Client client.Client
	Clock  clock.PassiveClock
}

// Reconcile reconciles a Lease object.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	lease := &coordinationv1.Lease{}
	if err := r.Client.Get(ctx, req.NamespacedName, lease); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	controllerRingName := lease.Labels[shardingv1alpha1.LabelControllerRing]
	if controllerRingName == "" {
		log.V(1).Info("Ignoring non-shard lease")
		return reconcile.Result{}, nil
	}

	if err := r.Client.Get(ctx, client.ObjectKey{Name: controllerRingName}, &shardingv1alpha1.ControllerRing{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("error checking for existence of ControllerRing: %w", err)
		}

		log.V(1).Info("Ignoring shard lease without a corresponding ControllerRing")
		return reconcile.Result{}, nil
	}

	var (
		now           = r.Clock.Now()
		previousState = leases.StateFromString(lease.Labels[shardingv1alpha1.LabelState])
		shard         = leases.ToShard(lease, now)
	)
	log = log.WithValues("state", shard.State, "expirationTime", shard.Times.Expiration, "leaseDuration", shard.Times.LeaseDuration)

	// maintain state label
	if previousState != shard.State {
		patch := client.MergeFromWithOptions(lease.DeepCopy(), client.MergeFromWithOptimisticLock{})
		metav1.SetMetaDataLabel(&lease.ObjectMeta, shardingv1alpha1.LabelState, shard.State.String())
		if err := r.Client.Patch(ctx, lease, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update state label on lease: %w", err)
		}
	}

	// act on state and determine when to check again
	var requeueAfter time.Duration
	switch shard.State {
	case leases.Ready:
		if previousState != leases.Ready {
			log.Info("Shard got available")
		}
		requeueAfter = shard.Times.ToExpired
	case leases.Expired:
		log.Info("Shard lease has expired")
		requeueAfter = shard.Times.ToUncertain
	case leases.Uncertain:
		log.Info("Shard lease has expired more than leaseDuration ago, trying to acquire shard lease")

		nowMicro := metav1.NewMicroTime(now)
		transitions := int32(0)
		if lease.Spec.LeaseTransitions != nil {
			transitions = *lease.Spec.LeaseTransitions
		}

		lease.Spec.HolderIdentity = ptr.To(shardingv1alpha1.IdentityShardLeaseController)
		lease.Spec.LeaseDurationSeconds = ptr.To(2 * int32(shard.Times.LeaseDuration.Round(time.Second).Seconds()))
		lease.Spec.AcquireTime = &nowMicro
		lease.Spec.RenewTime = &nowMicro
		lease.Spec.LeaseTransitions = ptr.To(transitions + 1)
		if err := r.Client.Update(ctx, lease); err != nil {
			return reconcile.Result{}, fmt.Errorf("error acquiring shard lease: %w", err)
		}

		// lease will be enqueued once we observe our previous update via watch
		// requeue with leaseDuration just to be sure
		requeueAfter = shard.Times.LeaseDuration
	case leases.Dead:
		if previousState != leases.Dead {
			log.Info("Shard got unavailable")
		}

		// garbage collect later
		requeueAfter = shard.Times.ToOrphaned
	case leases.Orphaned:
		// garbage collect and forget orphaned leases
		return reconcile.Result{}, r.Client.Delete(ctx, lease)
	default:
		// Unknown, forget lease
		return reconcile.Result{}, nil
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
}
