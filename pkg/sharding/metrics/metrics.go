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
	// AssignmentsTotal is a prometheus counter metric which holds the total number of shard assignments by the sharder
	// webhook per Ring and GroupResource.
	// It has a label which refers to the ControllerRing and two labels which refer to the object's GroupResource.
	AssignmentsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_sharding_assignments_total",
		Help: "Total number of shard assignments by the sharder webhook per Ring and GroupResource",
	}, []string{"ringName", "group", "resource"})

	// MovementsTotal is a prometheus counter metric which holds the total number of shard movements triggered by the
	// sharder controller per Ring and GroupResource.
	// It has a label which refers to the ControllerRing and two labels which refer to the object's GroupResource.
	MovementsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_sharding_movements_total",
		Help: "Total number of shard movements triggered by the sharder controller per Ring and GroupResource",
	}, []string{"ringName", "group", "resource"})

	// DrainsTotal is a prometheus counter metric which holds the total number of shard drains triggered by the sharder
	// controller per Ring and GroupResource.
	// It has a label which refers to the ControllerRing and two labels which refer to the object's GroupResource.
	DrainsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_sharding_drains_total",
		Help: "Total number of shard drains triggered by the sharder controller per Ring and GroupResource",
	}, []string{"ringName", "group", "resource"})

	// RingCalculationsTotal is a prometheus counter metric which holds the total
	// number of shard ring calculations per ring kind.
	// It has a label which refers to the ControllerRing.
	RingCalculationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "controller_sharding_ring_calculations_total",
		Help: "Total number of shard ring calculations per ring kind",
	}, []string{"name"})
)

func init() {
	metrics.Registry.MustRegister(
		AssignmentsTotal,
		MovementsTotal,
		DrainsTotal,
		RingCalculationsTotal,
	)
}
