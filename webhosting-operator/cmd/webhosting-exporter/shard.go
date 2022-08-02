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

package main

import (
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	"k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	"sigs.k8s.io/controller-runtime/pkg/controller/sharding"
)

const shardSubsystem = kubeStateMetricsPrefix + "shard_"

var (
	// shardCommonLabels are labels added on each shard metric
	shardCommonLabels = []string{"namespace", "shard", "app"}
)

type shardFactory struct {
	runtimeClientFactory
}

func newShardFactory() customresource.RegistryFactory {
	r, err := labels.NewRequirement(sharding.ShardLabel, selection.Exists, nil)
	utilruntime.Must(err)

	return shardFactory{
		runtimeClientFactory{
			listType:      &coordinationv1.LeaseList{},
			labelSelector: labels.NewSelector().Add(*r),
		},
	}
}

func (w shardFactory) Name() string {
	return "shards"
}

func (w shardFactory) ExpectedType() interface{} {
	return &coordinationv1.Lease{}
}

func (w shardFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		shardInfo(),
		shardState(),
	}
}

func shardInfo() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		shardSubsystem+"info",
		"Information about a Shard.",
		metric.Gauge,
		"",
		wrapShardFunc(func(lease *coordinationv1.Lease) *metric.Family {
			return &metric.Family{
				Metrics: []*metric.Metric{{
					LabelKeys:   []string{},
					LabelValues: []string{},
					Value:       1,
				}},
			}
		}),
	)
}

func shardState() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		shardSubsystem+"state",
		"The Shard's current state.",
		metric.Gauge,
		"",
		wrapShardFunc(func(lease *coordinationv1.Lease) *metric.Family {
			state := lease.Labels["state"]
			if state == "" {
				state = "unknown"
			}

			states := []string{
				"orphaned",
				"dead",
				"uncertain",
				"expired",
				"ready",
				"unknown",
			}

			ms := make([]*metric.Metric, len(states))
			for i, s := range states {
				ms[i] = &metric.Metric{
					LabelKeys:   []string{"state"},
					LabelValues: []string{s},
					Value:       boolValue(state == s),
				}
			}

			return &metric.Family{
				Metrics: ms,
			}
		}),
	)
}

func wrapShardFunc(f func(*coordinationv1.Lease) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		lease := obj.(*coordinationv1.Lease)
		metricFamily := f(lease)

		// populate common labels with values
		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(shardCommonLabels, m.LabelKeys...)
			m.LabelValues = append([]string{lease.Namespace, lease.Name, lease.Labels[sharding.ShardLabel]}, m.LabelValues...)
		}

		return metricFamily
	}
}
