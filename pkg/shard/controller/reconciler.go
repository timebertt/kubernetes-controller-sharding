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

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler wraps another reconciler to ensure that the controller correctly handles the shard and drain labels.
// It ignores any objects that are not assigned to the configured shard name and drains objects if instructed to by
// the sharder.
// You can use NewShardedReconciler to construct a sharded reconciler.
type Reconciler struct {
	// Object is the object type of the controller.
	Object client.Object
	// Client is used to read and patch the controller's objects.
	Client client.Client
	// ShardName is the shard ID of the manager.
	ShardName string
	// LabelShard is the shard label specific to the manager's ControllerRing.
	LabelShard string
	// LabelDrain is the drain label specific to the manager's ControllerRing.
	LabelDrain string
	// Do is the actual Reconciler.
	Do reconcile.Reconciler
}

// Reconcile implements reconcile.Reconciler. It performs the following steps:
// 1) read the object using the client, forget the object if it is not found
// 2) check whether the object is (still) assigned to this shard, forget the object otherwise
// 3) acknowledge the drain operation if requested by removing the shard and drain label, skip reconciliation, and forget the object
// 4) delegate reconciliation to the actual reconciler otherwise
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	obj := r.Object.DeepCopyObject().(client.Object)
	if err := r.Client.Get(ctx, request.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone or not assigned to this shard anymore, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store for determining responsibility: %w", err)
	}

	labels := obj.GetLabels()

	// check if we are responsible for this object
	// Note that objects should already be filtered by the cache and the predicate for being assigned to this shard.
	// However, we still need to do a final check before reconciling here. The controller might requeue the object with
	// a delay or exponential. This might trigger another reconciliation even after observing a label change.
	if shard, ok := labels[r.LabelShard]; !ok || shard != r.ShardName {
		log.V(1).Info("Ignoring object as it is assigned to different shard", "shard", shard)
		return reconcile.Result{}, nil
	}

	if _, drain := labels[r.LabelDrain]; drain {
		log.V(1).Info("Draining object")

		// acknowledge drain operation
		patch := client.MergeFromWithOptions(obj.DeepCopyObject().(client.Object), client.MergeFromWithOptimisticLock{})
		delete(labels, r.LabelShard)
		delete(labels, r.LabelDrain)

		if err := r.Client.Patch(ctx, obj, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("error draining object: %w", err)
		}

		// forget object
		return reconcile.Result{}, nil
	}

	// we are responsible, reconcile object
	return r.Do.Reconcile(ctx, request)
}
