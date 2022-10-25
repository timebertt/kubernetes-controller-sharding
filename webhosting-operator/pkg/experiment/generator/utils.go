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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log

// StartN adds a source to the given controller that emits exactly n events/reconcile.Request.
func StartN(c controller.Controller, n int) error {
	ch := make(chan event.GenericEvent, n)
	for i := 0; i < n; i++ {
		ch <- event.GenericEvent{Object: &metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{
				// use different object names, otherwise key will merge the requests
				Name: fmt.Sprintf("request-%d", n),
			},
		},
		}
	}

	return c.Watch(
		&source.Channel{Source: ch, DestBufferSize: 1},
		&handler.Funcs{GenericFunc: func(e event.GenericEvent, q workqueue.RateLimitingInterface) {
			q.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(e.Object)})
		}},
	)
}
