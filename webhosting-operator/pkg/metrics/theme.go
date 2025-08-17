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

	"github.com/timebertt/kubernetes-controller-sharding/pkg/metrics/exporter"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

var ThemeExporter = exporter.Exporter[*webhostingv1alpha1.Theme, *webhostingv1alpha1.ThemeList]{
	Namespace: namespace,
	Subsystem: "theme",

	StaticLabelKeys: []string{"theme", "uid"},
	GenerateStaticLabelValues: func(theme *webhostingv1alpha1.Theme) []string {
		return []string{theme.Name, string(theme.UID)}
	},

	Metrics: []exporter.Metric[*webhostingv1alpha1.Theme]{
		{
			Name:      "info",
			Help:      "Information about a Theme",
			LabelKeys: []string{"color", "font_family"},

			Generate: func(desc *prometheus.Desc, theme *webhostingv1alpha1.Theme, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					1,
					append(staticLabelValues, theme.Spec.Color, theme.Spec.FontFamily)...,
				)
			},
		},
		{
			Name: "metadata_generation",
			Help: "The generation of a Theme",

			Generate: func(desc *prometheus.Desc, theme *webhostingv1alpha1.Theme, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(theme.Generation),
					staticLabelValues...,
				)
			},
		},
	},
}
