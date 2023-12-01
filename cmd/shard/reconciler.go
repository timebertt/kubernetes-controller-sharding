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

package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardcontroller "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/controller"
)

// Reconciler is a reconciler for ConfigMaps that creates a dummy secret for every ConfigMap.
// It handles the shard and drain label.
type Reconciler struct {
	Client client.Client
}

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager, clusterRingName, shardName string) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	// ACKNOWLEDGE DRAIN OPERATIONS
	// Use the shardcontroller package as helpers for:
	// - a predicate that triggers when the drain label is present (even if the actual predicates don't trigger)
	// - wrapping the actual reconciler a reconciler that handles the drain operation for us
	return builder.ControllerManagedBy(mgr).
		Named("configmap").
		For(&corev1.ConfigMap{}, builder.WithPredicates(shardcontroller.Predicate(clusterRingName, shardName, ConfigMapDataChanged(), predicate.GenerationChangedPredicate{}))).
		Owns(&corev1.Secret{}, builder.WithPredicates(ObjectDeleted())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(
			shardcontroller.NewShardedReconciler(mgr).
				For(&corev1.ConfigMap{}).
				InClusterRing(clusterRingName).
				WithShardName(shardName).
				MustBuild(r),
		)
}

// ConfigMapDataChanged returns a predicate that is similar to predicate.GenerationChangedPredicate but for ConfigMaps
// that don't have a metadata.generation field.
func ConfigMapDataChanged() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return apiequality.Semantic.DeepEqual(e.ObjectOld.(*corev1.ConfigMap).Data, e.ObjectNew.(*corev1.ConfigMap).Data)
		},
	}
}

// ObjectDeleted returns a predicate that only triggers for DELETE events.
func ObjectDeleted() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(_ event.CreateEvent) bool { return false },
		UpdateFunc:  func(_ event.UpdateEvent) bool { return false },
		DeleteFunc:  func(_ event.DeleteEvent) bool { return true },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}

// Reconcile reconciles a ConfigMap.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, req.NamespacedName, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	// perform some dummy operation, this is just an example controller, we don't really care what it actually does
	// create a secret with controller reference so that the controller also serves as an example with controlled objects
	log.V(1).Info("Reconciling object")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dummy-" + configMap.Name,
			Namespace: configMap.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(configMap, secret, r.Client.Scheme()); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, client.IgnoreAlreadyExists(r.Client.Create(ctx, secret))
}
