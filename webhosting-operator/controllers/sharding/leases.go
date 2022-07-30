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
	"strings"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/sharding/leases"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/consistenthash"
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

	Ring *consistenthash.Ring
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

	state := leases.ToShardState(lease, r.Clock)
	log = log.WithValues("state", state)

	// update state label if required
	if currentState := leases.ShardStateFromString(lease.Labels[stateLabel]); currentState != state {
		patch := client.MergeFromWithOptions(lease.DeepCopy(), client.MergeFromWithOptimisticLock{})
		metav1.SetMetaDataLabel(&lease.ObjectMeta, stateLabel, string(state))
		if err := r.Client.Patch(ctx, lease, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update state label on lease: %w", err)
		}
	}

	// prepare requeue after durations
	expirationTime, leaseDuration, ok := leases.ExpirationTime(lease)
	if !ok {
		expirationTime = r.Clock.Now()
		leaseDuration = defaultLeaseDuration
	}

	durationToExpiration := expirationTime.Sub(r.Clock.Now())
	durationToOrphaned := expirationTime.Add(leaseTTL).Sub(r.Clock.Now())

	// act on changes to actual state of world if any
	switch state {
	case leases.Ready:
		if r.Ring.AddNode(lease.Name) {
			log.Info("Added ready shard to ring")
		}
		return reconcile.Result{RequeueAfter: durationToExpiration}, nil
	case leases.Uncertain:
		log = log.WithValues("expirationTime", expirationTime, "leaseDuration", leaseDuration)

		// shard is considered dead once renewTime + 2*leaseDuration has passed
		durationToDead := durationToExpiration + leaseDuration
		if durationToDead <= 0 {
			log.Info("Shard is dead, trying to acquire shard lease")

			patch := client.MergeFromWithOptions(lease.DeepCopy(), client.MergeFromWithOptimisticLock{})
			lease.Spec.HolderIdentity = pointer.String(sharderIdentity)
			return reconcile.Result{RequeueAfter: durationToOrphaned}, r.Client.Patch(ctx, lease, patch)
		}

		log.Info("Shard lease has expired")
		return reconcile.Result{RequeueAfter: durationToDead}, nil
	case leases.Dead:
		if r.Ring.RemoveNode(lease.Name) {
			log.Info("Removed dead shard from ring")
		}

		if durationToOrphaned <= 0 {
			// garbage collect orphaned leases
			return reconcile.Result{}, r.Client.Delete(ctx, lease)
		}

		// garbage collect later
		return reconcile.Result{RequeueAfter: durationToOrphaned}, nil
	default: // Unknown
		if r.Ring.RemoveNode(lease.Name) {
			log.Info("Removed node from ring")
		}

		return reconcile.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *leaseReconciler) SetupWithManager(mgr manager.Manager) error {
	return builder.ControllerManagedBy(mgr).
		Named("sharder").
		For(&coordinationv1.Lease{}, builder.WithPredicates(leasePredicate(r.LeaseNamespace))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RecoverPanic:            true,
		}).
		Complete(r)
}

func leasePredicate(namespace string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		// TODO: fix this
		return object.GetNamespace() == namespace && strings.HasPrefix(object.GetName(), "webhosting-operator-")
	})
}
