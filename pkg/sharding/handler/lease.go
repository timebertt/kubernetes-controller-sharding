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

package handler

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// MapLeaseToControllerRing maps a shard lease to its ControllerRing.
func MapLeaseToControllerRing(_ context.Context, obj client.Object) []reconcile.Request {
	ring := obj.GetLabels()[shardingv1alpha1.LabelControllerRing]
	if ring == "" {
		return nil
	}

	return []reconcile.Request{{NamespacedName: client.ObjectKey{Name: ring}}}
}
