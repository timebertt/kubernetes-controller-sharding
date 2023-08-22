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

package generator

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log

// EmitN returns a source that emits exactly n events (reconcile.Request). The source ignores predicates.
// Use it with the controller builder:
//
//	WatchesRawSource(EmitN(n), &handler.EnqueueRequestForObject{})
//
// Or a plain controller:
//
//	Watch(EmitN(n), &handler.EnqueueRequestForObject{})
func EmitN(n int) source.Source {
	return source.Func(func(ctx context.Context, eventHandler handler.EventHandler, queue workqueue.RateLimitingInterface, _ ...predicate.Predicate) error {
		for i := 0; i < n; i++ {
			ev := event.GenericEvent{Object: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					// use different object names, otherwise queue will merge the requests
					Name: fmt.Sprintf("request-%d", n),
				},
			}}

			eventHandler.Generic(ctx, ev, queue)
		}

		return nil
	})
}

// CreateClusterScopedOwnerObject creates a new cluster-scoped object that has a single purpose: being used as an owner
// for multiple objects that should be cleaned up at once. This is useful for cleaning up a lot of objects (of different
// kinds) at once with a single DELETE call.
func CreateClusterScopedOwnerObject(ctx context.Context, c client.Client, opts ...GenerateOption) (client.Object, *metav1.OwnerReference, error) {
	options := (&GenerateOptions{}).ApplyOptions(opts...)

	ownerObject := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "experiment-owner-",
		},
	}
	options.ApplyToObject(&ownerObject.ObjectMeta)

	if err := c.Create(ctx, ownerObject); err != nil {
		return nil, nil, err
	}

	return ownerObject, metav1.NewControllerRef(ownerObject, rbacv1.SchemeGroupVersion.WithKind("ClusterRole")), nil
}
