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
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log

// StartOnce adds a source to the given controller that emits exactly one event/reconcile.Request.
func StartOnce(c controller.Controller) error {
	ch := make(chan event.GenericEvent, 1)
	ch <- event.GenericEvent{}

	return c.Watch(
		&source.Channel{Source: ch, DestBufferSize: 1},
		&handler.Funcs{GenericFunc: func(_ event.GenericEvent, q workqueue.RateLimitingInterface) { q.Add(reconcile.Request{}) }},
	)
}
