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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// AssignmentsTotal is a prometheus counter metric which holds the total
	// number of shard assignments per GroupKind.
	// It has two labels which refer to the object's GroupKind and two labels
	// which refer to the controller's GroupKind.
	AssignmentsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sharding_assignments_total",
		Help: "Total number of shard assignments per GroupKind",
	}, []string{"group", "kind", "controllerGroup", "controllerKind"})

	// DrainsTotal is a prometheus counter metric which holds the total
	// number of shard drains per GroupKind.
	// It has two labels which refer to the object's GroupKind and two labels
	// which refer to the controller's GroupKind.
	DrainsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sharding_drains_total",
		Help: "Total number of shard drains per GroupKind",
	}, []string{"group", "kind", "controllerGroup", "controllerKind"})

	// RingCalculationsTotal is a prometheus counter metric which holds the total
	// number of shard ring calculations per ring kind.
	// It has three labels which refer to the ring's kind, name, and namespace.
	RingCalculationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sharding_ring_calculations_total",
		Help: "Total number of shard ring calculations per GroupKind",
	}, []string{"kind", "namespace", "name"})
)

func init() {
	metrics.Registry.MustRegister(
		AssignmentsTotal,
		DrainsTotal,
		RingCalculationsTotal,
	)
}
