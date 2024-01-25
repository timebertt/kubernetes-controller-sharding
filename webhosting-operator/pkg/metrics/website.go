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

const websiteSubsystem = "website"

type WebsiteExporter struct {
	client.Reader
}

func (e *WebsiteExporter) AddToManager(mgr manager.Manager) error {
	if e.Reader == nil {
		e.Reader = mgr.GetShardedCache()
	}

	return mgr.Add(e)
}

// Start registers this collector in the controller-runtime metrics registry.
// When Start runs, caches have already been started, so we are ready to export metrics.
func (e *WebsiteExporter) Start(_ context.Context) error {
	if err := metrics.Registry.Register(e); err != nil {
		return fmt.Errorf("failed to register website exporter: %w", err)
	}

	return nil
}

func (e *WebsiteExporter) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range websiteMetrics {
		ch <- desc.desc
	}
}

func (e *WebsiteExporter) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	websiteList := &webhostingv1alpha1.WebsiteList{}
	if err := e.List(ctx, websiteList); err != nil {
		for _, desc := range websiteMetrics {
			ch <- prometheus.NewInvalidMetric(desc.desc, fmt.Errorf("error listing websites: %w", err))
		}

		return
	}

	for _, item := range websiteList.Items {
		website := item

		staticLabels := generateWebsiteStaticLabels(&website)

		for _, desc := range websiteMetrics {
			desc.generate(desc.desc, &website, staticLabels, ch)
		}
	}
}

var (
	websiteMetrics = []metric[*webhostingv1alpha1.Website]{
		websiteInfo,
		websiteShard,
		websiteGeneration,
		websiteObservedGeneration,
		websiteStatusPhase,
	}

	websiteStaticLabels = []string{
		"namespace",
		"website",
		"uid",
	}
)

func generateWebsiteStaticLabels(website *webhostingv1alpha1.Website) []string {
	return []string{
		website.Namespace,
		website.Name,
		string(website.UID),
	}
}

var (
	websiteInfo = metric[*webhostingv1alpha1.Website]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, websiteSubsystem, "info"),
			"Information about a Website",
			append(websiteStaticLabels, "theme"),
			nil,
		),

		generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabels []string, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				1,
				append(staticLabels, website.Spec.Theme)...,
			)
		},
	}

	websiteShard = metric[*webhostingv1alpha1.Website]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, websiteSubsystem, "shard"),
			"Sharding information about a Website",
			append(websiteStaticLabels, "shard", "drain"),
			nil,
		),

		generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabels []string, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				1,
				append(staticLabels,
					website.Labels["shard.alpha.sharding.timebertt.dev/clusterring-ef3d63cd-webhosting-operator"],
					website.Labels["drain.alpha.sharding.timebertt.dev/clusterring-ef3d63cd-webhosting-operator"],
				)...,
			)
		},
	}

	websiteGeneration = metric[*webhostingv1alpha1.Website]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, websiteSubsystem, "metadata_generation"),
			"The generation of a Website",
			websiteStaticLabels,
			nil,
		),

		generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabels []string, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				float64(website.Generation),
				staticLabels...,
			)
		},
	}

	websiteObservedGeneration = metric[*webhostingv1alpha1.Website]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, websiteSubsystem, "observed_generation"),
			"The latest generation observed by the Website controller",
			websiteStaticLabels,
			nil,
		),

		generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabels []string, ch chan<- prometheus.Metric) {
			ch <- prometheus.MustNewConstMetric(
				desc,
				prometheus.GaugeValue,
				float64(website.Status.ObservedGeneration),
				staticLabels...,
			)
		},
	}

	websiteStatusPhase = metric[*webhostingv1alpha1.Website]{
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, websiteSubsystem, "status_phase"),
			"The Website's current phase (StateSet)",
			append(websiteStaticLabels, "phase"),
			nil,
		),

		generate: func(desc *prometheus.Desc, website *webhostingv1alpha1.Website, staticLabels []string, ch chan<- prometheus.Metric) {
			for _, phase := range webhostingv1alpha1.AllWebsitePhases {
				value := 0
				if website.Status.Phase == phase {
					value = 1
				}

				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					float64(value),
					append(staticLabels, string(phase))...,
				)
			}
		},
	}
)
