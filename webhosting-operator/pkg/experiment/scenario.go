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

package experiment

import (
	"fmt"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var registry = make(map[string]Scenario)

// Scenario is an evaluation scenario that can be executed by experiment.
type Scenario interface {
	// Name returns the name of the scenario.
	Name() string
	// Done is closed once the scenario is finished.
	Done() <-chan struct{}
	// AddToManager adds all runnables of the scenario to the manager.
	AddToManager(manager.Manager) error
}

// RegisterScenario registers a new scenario in the registry.
func RegisterScenario(s Scenario) {
	if _, ok := registry[s.Name()]; ok {
		panic(fmt.Errorf("scenario %q already registered", s.Name()))
	}

	registry[s.Name()] = s
}

// GetAllScenarios returns all registered scenarios.
func GetAllScenarios() []string {
	var all []string
	for s := range registry {
		all = append(all, s)
	}
	sort.Strings(all)
	return all
}

// GetScenario gets a single registered scenario by name.
func GetScenario(s string) Scenario {
	return registry[s]
}
