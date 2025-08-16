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

package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const Namespace = "controller_sharding"

// AddToManager adds all metrics exporters for sharding objects to the manager.
func AddToManager(mgr manager.Manager) error {
	if err := ControllerRingExporter.AddToManager(mgr); err != nil {
		return fmt.Errorf("failed to add ControllerRing exporter: %w", err)
	}

	if err := ShardExporter.AddToManager(mgr); err != nil {
		return fmt.Errorf("failed to add shard exporter: %w", err)
	}

	for _, collector := range []prometheus.Collector{
		AssignmentsTotal,
		MovementsTotal,
		DrainsTotal,
		RingCalculationsTotal,
	} {
		if err := metrics.Registry.Register(collector); err != nil {
			return fmt.Errorf("failed to add sharding operations metrics: %w", err)
		}
	}

	return nil
}
