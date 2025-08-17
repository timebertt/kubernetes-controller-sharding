/*
Copyright 2024 Tim Ebert.

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
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

var WebsiteExporter = exporter.Exporter[*webhostingv1alpha1.Website, *webhostingv1alpha1.WebsiteList]{
	Namespace: namespace,
	Subsystem: "website",

	StaticLabelKeys: []string{"namespace", "website", "uid"},
	GenerateStaticLabelValues: func(website *webhostingv1alpha1.Website) []string {
		return []string{website.Namespace, website.Name, string(website.UID)}
	},

	Metrics: []exporter.Metric[*webhostingv1alpha1.Website]{
		{
			Name:      "info",
			Help:      "Information about a Website",
			LabelKeys: []string{"theme"},

			Generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					1,
					append(staticLabelValues, website.Spec.Theme)...,
				)
			},
		},
		{
			Name:      "shard",
			Help:      "Sharding information about a Website",
			LabelKeys: []string{"shard", "drain"},

			Generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					1,
					append(staticLabelValues,
						website.Labels[shardingv1alpha1.LabelShard(webhostingv1alpha1.WebhostingOperatorName)],
						website.Labels[shardingv1alpha1.LabelDrain(webhostingv1alpha1.WebhostingOperatorName)],
					)...,
				)
			},
		},
		{
			Name: "metadata_generation",
			Help: "The generation of a Website",

			Generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(website.Generation),
					staticLabelValues...,
				)
			},
		},
		{
			Name: "observed_generation",
			Help: "The latest generation observed by the Website controller",

			Generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(website.Status.ObservedGeneration),
					staticLabelValues...,
				)
			},
		},
		{
			Name:      "status_phase",
			Help:      "The Website's current phase (StateSet)",
			LabelKeys: []string{"phase"},

			Generate: exporter.GenerateStateSet[*webhostingv1alpha1.Website](
				exporter.KnownStates(webhostingv1alpha1.AllWebsitePhases), nil,
				func(website *webhostingv1alpha1.Website) string {
					return string(website.Status.Phase)
				},
			),
		},
	},
}
