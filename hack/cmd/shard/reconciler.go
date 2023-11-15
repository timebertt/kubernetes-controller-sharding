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

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// Reconciler is a reconciler for ConfigMaps that only handles the shard and drain label but doesn't actually do
// anything useful with the reconciled objects.
type Reconciler struct {
	Client client.Client

	ClusterRingName string
	ShardName       string

	labelShard, labelDrain string
}

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}

	r.labelShard = shardingv1alpha1.LabelShard(shardingv1alpha1.KindClusterRing, "", r.ClusterRingName)
	r.labelDrain = shardingv1alpha1.LabelDrain(shardingv1alpha1.KindClusterRing, "", r.ClusterRingName)

	return builder.ControllerManagedBy(mgr).
		Named("configmap").
		For(&corev1.ConfigMap{}, builder.WithPredicates(ConfigMapDataChanged())).
		Owns(&corev1.Secret{}, builder.WithPredicates(ObjectDeleted())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
		}).
		Complete(r)
}

func ConfigMapDataChanged() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			switch objNew := e.ObjectNew.(type) {
			case *corev1.ConfigMap:
				objOld := e.ObjectOld.(*corev1.ConfigMap)
				return apiequality.Semantic.DeepEqual(objOld.Data, objNew.Data)
			case *corev1.Secret:
				objOld := e.ObjectOld.(*corev1.Secret)
				return apiequality.Semantic.DeepEqual(objOld.Data, objNew.Data)
			}

			return false
		},
	}
}

func ObjectDeleted() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return false },
		UpdateFunc: func(_ event.UpdateEvent) bool { return false },
		DeleteFunc: func(_ event.DeleteEvent) bool { return true },
	}
}

// Reconcile reconciles a ClusterRing object.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, req.NamespacedName, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone or not assigned to this shard anymore, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store for determining responsibility: %w", err)
	}

	labels := configMap.GetLabels()

	// check if we are responsible for this object
	if shard, ok := labels[r.labelShard]; !ok || shard != r.ShardName {
		// should not happen usually because of filtered watch
		log.V(1).Info("Ignoring object as it is assigned to different shard", "shard", shard)
		return reconcile.Result{}, nil
	}

	if _, drain := labels[r.labelDrain]; drain {
		log.V(1).Info("Draining object")

		// acknowledge drain operation
		patch := client.MergeFromWithOptions(configMap.DeepCopy(), client.MergeFromWithOptimisticLock{})
		delete(labels, r.labelShard)
		delete(labels, r.labelDrain)

		if err := r.Client.Patch(ctx, configMap, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("error draining object: %w", err)
		}

		// forget object
		return reconcile.Result{}, nil
	}

	// we are responsible, reconcile object
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
