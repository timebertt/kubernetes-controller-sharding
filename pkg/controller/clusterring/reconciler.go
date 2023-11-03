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

package clusterring

import (
	"context"
	"fmt"

	coordinationv1 "k8s.io/api/coordination/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
)

//+kubebuilder:rbac:groups=sharding.timebertt.dev,resources=clusterrings,verbs=get;list;watch
//+kubebuilder:rbac:groups=sharding.timebertt.dev,resources=clusterrings/status,verbs=update;patch

// Reconciler reconciles ClusterRings.
type Reconciler struct {
	Client client.Client
	Clock  clock.PassiveClock
}

// Reconcile reconciles a ClusterRing object.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	clusterRing := &shardingv1alpha1.ClusterRing{}
	if err := r.Client.Get(ctx, req.NamespacedName, clusterRing); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	clusterRingCopy := clusterRing.DeepCopy()

	// update status with the latest observed generation
	clusterRing.Status.ObservedGeneration = clusterRing.Generation

	leaseList := &coordinationv1.LeaseList{}
	if err := r.Client.List(ctx, leaseList, client.MatchingLabels{shardingv1alpha1.LabelClusterRing: clusterRing.Name}); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing Leases for ClusterRing: %w", err)
	}

	shards := leases.ToShards(leaseList.Items, r.Clock)
	clusterRing.Status.Shards = int32(len(shards))
	clusterRing.Status.AvailableShards = int32(len(shards.AvailableShards()))

	if !apiequality.Semantic.DeepEqual(clusterRing.Status, clusterRingCopy.Status) {
		if err := r.Client.Status().Update(ctx, clusterRing); err != nil {
			return reconcile.Result{}, fmt.Errorf("error updating ClusterRing status: %w", err)
		}
	}

	return reconcile.Result{}, nil
}
