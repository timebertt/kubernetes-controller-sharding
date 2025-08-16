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
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/metrics/exporter"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
)

var ShardExporter = exporter.Exporter[*coordinationv1.Lease, *coordinationv1.LeaseList]{
	Namespace: Namespace,
	Subsystem: "shard",

	ListOptions: []client.ListOption{client.HasLabels{shardingv1alpha1.LabelControllerRing}},

	StaticLabelKeys: []string{"namespace", "shard", "controllerring"},
	GenerateStaticLabelValues: func(lease *coordinationv1.Lease) []string {
		return []string{lease.Namespace, lease.Name, lease.Labels[shardingv1alpha1.LabelControllerRing]}
	},

	Metrics: []exporter.Metric[*coordinationv1.Lease]{
		{
			Name:      "info",
			Help:      "Information about a shard",
			LabelKeys: []string{"uid"},

			Generate: func(desc *prometheus.Desc, lease *coordinationv1.Lease, staticLabelValues []string, ch chan<- prometheus.Metric) {
				ch <- prometheus.MustNewConstMetric(
					desc,
					prometheus.GaugeValue,
					1,
					append(staticLabelValues, string(lease.UID))...,
				)
			},
		},
		{
			Name:      "state",
			Help:      "The shard's current state observed by the shardlease controller",
			LabelKeys: []string{"state"},

			Generate: exporter.GenerateStateSet[*coordinationv1.Lease](
				exporter.KnownStatesStringer(leases.KnownShardStates), ptr.To(leases.Unknown.String()),
				func(lease *coordinationv1.Lease) string {
					return lease.Labels[shardingv1alpha1.LabelState]
				},
			),
		},
	},
}
