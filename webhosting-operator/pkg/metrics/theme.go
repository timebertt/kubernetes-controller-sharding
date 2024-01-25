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
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

const themeSubsystem = "theme"

type ThemeExporter struct {
	client.Reader
}

func (e *ThemeExporter) AddToManager(mgr manager.Manager) error {
	if e.Reader == nil {
		e.Reader = mgr.GetCache()
	}

	return mgr.Add(e)
}

// Start registers this collector in the controller-runtime metrics registry.
// When Start runs, caches have already been started, so we are ready to export metrics.
func (e *ThemeExporter) Start(_ context.Context) error {
	if err := metrics.Registry.Register(e); err != nil {
		return fmt.Errorf("failed to register theme exporter: %w", err)
	}

	return nil
}

func (e *ThemeExporter) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range themeMetrics {
		ch <- desc.desc
	}
}

func (e *ThemeExporter) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	themeList := &webhostingv1alpha1.ThemeList{}
	if err := e.List(ctx, themeList); err != nil {
		for _, desc := range themeMetrics {
			ch <- prometheus.NewInvalidMetric(desc.desc, fmt.Errorf("error listing themes: %w", err))
		}

		return
	}

	for _, item := range themeList.Items {
		theme := item

		staticLabels := generateThemeStaticLabels(&theme)

		for _, desc := range themeMetrics {
			desc.generate(desc.desc, &theme, staticLabels, ch)
		}
	}
}

var (
	themeMetrics = []metric[*webhostingv1alpha1.Theme]{
		themeInfo,
		themeGeneration,
	}

	themeStaticLabels = []string{
		"theme",
		"uid",
	}
)

func generateThemeStaticLabels(theme *webhostingv1alpha1.Theme) []string {
	return []string{
		theme.Name,
		string(theme.UID),
	}
}

var (
	themeInfo = metric[*webhostingv1alpha1.Theme]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, themeSubsystem, "info"),
			"Information about a Theme",
			append(themeStaticLabels, "color", "font_family"),
			nil,
		),

		generate: func(desc *prometheus.Desc, theme *webhostingv1alpha1.Theme, staticLabels []string, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				1,
				append(staticLabels, theme.Spec.Color, theme.Spec.FontFamily)...,
			)
		},
	}

	themeGeneration = metric[*webhostingv1alpha1.Theme]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, themeSubsystem, "metadata_generation"),
			"The generation of a Theme",
			themeStaticLabels,
			nil,
		),

		generate: func(desc *prometheus.Desc, theme *webhostingv1alpha1.Theme, staticLabels []string, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				float64(theme.Generation),
				staticLabels...,
			)
		},
	}
)
