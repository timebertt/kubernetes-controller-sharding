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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
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
