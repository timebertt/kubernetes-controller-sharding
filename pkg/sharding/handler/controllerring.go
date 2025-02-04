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

	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

var handlerLog = logf.Log.WithName("handler")

// MapControllerRingToLeases maps a ControllerRing to all matching shard leases.
func MapControllerRingToLeases(reader client.Reader) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		controllerRing, ok := obj.(*shardingv1alpha1.ControllerRing)
		if !ok {
			return nil
		}

		leaseList := &coordinationv1.LeaseList{}
		if err := reader.List(ctx, leaseList, client.MatchingLabelsSelector{Selector: controllerRing.LeaseSelector()}); err != nil {
			handlerLog.Error(err, "failed listing Leases for ControllerRing", "controllerRing", client.ObjectKeyFromObject(controllerRing))
			return nil
		}

		requests := make([]reconcile.Request, 0, len(leaseList.Items))
		for _, lease := range leaseList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&lease)})
		}

		return requests
	}
}
