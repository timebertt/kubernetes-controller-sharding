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
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/sharding/leases"
)

const (
	sharderIdentity = "sharder"
	stateLabel      = "state"
)

// leaseReconciler reconciles shard leases.
type leaseReconciler struct {
	Client client.Client
	Clock  clock.Clock

	LeaseNamespace string
}

//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;update;patch;delete

// Reconcile reconciles a Lease object.
func (r *leaseReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	lease := &coordinationv1.Lease{}
	if err := r.Client.Get(ctx, req.NamespacedName, lease); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	var (
		previousState = leases.StateFromString(lease.Labels[stateLabel])
		shard         = leases.ToShard(lease, r.Clock)
	)
	log = log.WithValues("state", shard.State, "expirationTime", shard.Times.Expiration, "leaseDuration", shard.Times.LeaseDuration)

	// maintain state label
	if previousState != shard.State {
		patch := client.MergeFromWithOptions(lease.DeepCopy(), client.MergeFromWithOptimisticLock{})
		metav1.SetMetaDataLabel(&lease.ObjectMeta, stateLabel, shard.State.String())
		if err := r.Client.Patch(ctx, lease, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update state label on lease: %w", err)
		}
	}

	// act on state and determine when to check again
	var requeueAfter time.Duration
	switch shard.State {
	case leases.Ready:
		if previousState != leases.Ready {
			log.Info("Shard got ready")
		}
		requeueAfter = shard.Times.ToExpired
	case leases.Expired:
		log.Info("Shard lease has expired")
		requeueAfter = shard.Times.ToUncertain
	case leases.Uncertain:
		log.Info("Shard lease has expired more than leaseDuration ago, trying to acquire shard lease")

		now := metav1.NewMicroTime(r.Clock.Now())
		transitions := int32(0)
		if lease.Spec.LeaseTransitions != nil {
			transitions = *lease.Spec.LeaseTransitions
		}

		lease.Spec.HolderIdentity = pointer.String(sharderIdentity)
		lease.Spec.LeaseDurationSeconds = pointer.Int32(2 * int32(shard.Times.LeaseDuration.Round(time.Second).Seconds()))
		lease.Spec.AcquireTime = &now
		lease.Spec.RenewTime = &now
		lease.Spec.LeaseTransitions = pointer.Int32(transitions + 1)
		if err := r.Client.Update(ctx, lease); err != nil {
			return reconcile.Result{}, fmt.Errorf("error acquiring shard lease: %w", err)
		}

		// lease will be enqueued once we observe our previous update via watch
		// requeue with leaseDuration just to be sure
		requeueAfter = shard.Times.LeaseDuration
	case leases.Dead:
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

// SetupWithManager sets up the controller with the Manager.
func (r *leaseReconciler) SetupWithManager(mgr manager.Manager) error {
	return builder.ControllerManagedBy(mgr).
		Named("sharder").
		For(&coordinationv1.Lease{}, builder.WithPredicates(r.leasePredicate())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RecoverPanic:            true,
		}).
		Complete(r)
}

func (r *leaseReconciler) leasePredicate() predicate.Predicate {
	// ignore deletion of shard leases
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return true },
		UpdateFunc: func(_ event.UpdateEvent) bool { return true },
		DeleteFunc: func(_ event.DeleteEvent) bool { return false },
	}
}
