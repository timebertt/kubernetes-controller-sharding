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

package predicate

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ControllerRingCreatedOrUpdated reacts on create and update events with generation changes but ignores delete
// events. On deletion, there is nothing to do for the sharding controllers.
func ControllerRingCreatedOrUpdated() predicate.Predicate {
	return predicate.And(
		predicate.GenerationChangedPredicate{},
		// ignore deletion of ControllerRings
		predicate.Funcs{
			CreateFunc: func(_ event.CreateEvent) bool { return true },
			UpdateFunc: func(_ event.UpdateEvent) bool { return true },
			DeleteFunc: func(_ event.DeleteEvent) bool { return false },
		},
	)
}
