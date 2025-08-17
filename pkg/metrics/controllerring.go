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
	"github.com/prometheus/client_golang/prometheus"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/metrics/exporter"
)

var ControllerRingExporter = exporter.Exporter[*shardingv1alpha1.ControllerRing, *shardingv1alpha1.ControllerRingList]{
	Namespace: Namespace,
	Subsystem: "controllerring",

	StaticLabelKeys: []string{"controllerring", "uid"},
	GenerateStaticLabelValues: func(controllerRing *shardingv1alpha1.ControllerRing) []string {
		return []string{controllerRing.Name, string(controllerRing.UID)}
	},

	Metrics: []exporter.Metric[*shardingv1alpha1.ControllerRing]{
		{
			Name: "metadata_generation",
			Help: "The generation of a ControllerRing",

			Generate: func(desc *prometheus.Desc, controllerRing *shardingv1alpha1.ControllerRing, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(controllerRing.Generation),
					staticLabelValues...,
				)
			},
		},
		{
			Name: "observed_generation",
			Help: "The latest generation observed by the ControllerRing controller",

			Generate: func(desc *prometheus.Desc, controllerRing *shardingv1alpha1.ControllerRing, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(controllerRing.Status.ObservedGeneration),
					staticLabelValues...,
				)
			},
		},
		{
			Name: "status_shards",
			Help: "The ControllerRing's total number of shards observed by the ControllerRing controller",

			Generate: func(desc *prometheus.Desc, controllerRing *shardingv1alpha1.ControllerRing, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(controllerRing.Status.Shards),
					staticLabelValues...,
				)
			},
		},
		{
			Name: "status_available_shards",
			Help: "The ControllerRing's number of available shards observed by the ControllerRing controller",

			Generate: func(desc *prometheus.Desc, controllerRing *shardingv1alpha1.ControllerRing, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(controllerRing.Status.AvailableShards),
					staticLabelValues...,
				)
			},
		},
	},
}
