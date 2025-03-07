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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardcontroller "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/controller"
)

const (
	LabelKey          = "status"
	LabelValuePending = "pending"
	LabelValueDone    = "done"
	LabelValueIgnore  = "ignore"
)

// Reconciler watches ConfigMaps with the `status=pending` label and sets the `status` label to `done`.
type Reconciler struct {
	Client client.Client
}

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, controllerRingName, shardName string) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	return builder.ControllerManagedBy(mgr).
		Named("test").
		For(&corev1.ConfigMap{}, builder.WithPredicates(shardcontroller.Predicate(controllerRingName, shardName, ConfigMapPredicate()))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(
			shardcontroller.NewShardedReconciler(mgr).
				For(&corev1.ConfigMap{}).
				InControllerRing(controllerRingName).
				WithShardName(shardName).
				MustBuild(r),
		)
}

func ConfigMapPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetLabels()[LabelKey] == LabelValuePending
	})
}

// Reconcile reconciles a ConfigMap.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	secret := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, req.NamespacedName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	log.V(1).Info("Reconciling object")
	patch := client.MergeFrom(secret.DeepCopy())
	metav1.SetMetaDataLabel(&secret.ObjectMeta, LabelKey, LabelValueDone)
	return reconcile.Result{}, r.Client.Patch(ctx, secret, patch)
}
