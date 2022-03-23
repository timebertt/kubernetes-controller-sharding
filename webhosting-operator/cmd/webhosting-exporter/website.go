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
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	"k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/webhosting/v1alpha1"
)

const websiteSubsystem = kubeStateMetricsPrefix + "website_"

var (
	// websiteCommonLabels are labels added on each website metric
	websiteCommonLabels = []string{"namespace", "website"}
)

type websiteFactory struct {
	runtimeClientFactory
}

func newWebsiteFactory() customresource.RegistryFactory {
	return websiteFactory{
		runtimeClientFactory{
			listType: &webhostingv1alpha1.WebsiteList{},
		},
	}
}

func (w websiteFactory) Name() string {
	return "websites"
}

func (w websiteFactory) ExpectedType() interface{} {
	return &webhostingv1alpha1.Website{}
}

func (w websiteFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		websiteInfo(),
		websiteMetadataGeneration(),
		websiteStatusObservedGeneration(),
		websiteStatusPhase(),
	}
}

func websiteInfo() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		websiteSubsystem+"info",
		"Information about a Website.",
		metric.Gauge,
		"",
		wrapWebsiteFunc(func(w *webhostingv1alpha1.Website) *metric.Family {
			return &metric.Family{
				Metrics: []*metric.Metric{{
					LabelKeys:   []string{"theme"},
					LabelValues: []string{w.Spec.Theme},
					Value:       1,
				}},
			}
		}),
	)
}

func websiteMetadataGeneration() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		websiteSubsystem+"metadata_generation",
		"The generation of a Website.",
		metric.Gauge,
		"",
		wrapWebsiteFunc(func(w *webhostingv1alpha1.Website) *metric.Family {
			return &metric.Family{
				Metrics: []*metric.Metric{{
					Value: float64(w.ObjectMeta.Generation),
				}},
			}
		}),
	)
}

func websiteStatusObservedGeneration() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		websiteSubsystem+"observed_generation",
		"The latest generation observed by the Website controller.",
		metric.Gauge,
		"",
		wrapWebsiteFunc(func(w *webhostingv1alpha1.Website) *metric.Family {
			return &metric.Family{
				Metrics: []*metric.Metric{{
					Value: float64(w.Status.ObservedGeneration),
				}},
			}
		}),
	)
}

func websiteStatusPhase() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		websiteSubsystem+"status_phase",
		"The Website's current phase.",
		metric.Gauge,
		"",
		wrapWebsiteFunc(func(w *webhostingv1alpha1.Website) *metric.Family {
			phase := w.Status.Phase
			if phase == "" {
				return &metric.Family{
					Metrics: []*metric.Metric{},
				}
			}

			phases := []struct {
				v bool
				n string
			}{
				{phase == webhostingv1alpha1.PhasePending, string(webhostingv1alpha1.PhasePending)},
				{phase == webhostingv1alpha1.PhaseReady, string(webhostingv1alpha1.PhaseReady)},
				{phase == webhostingv1alpha1.PhaseError, string(webhostingv1alpha1.PhaseError)},
			}

			ms := make([]*metric.Metric, len(phases))
			for i, p := range phases {
				ms[i] = &metric.Metric{
					LabelKeys:   []string{"phase"},
					LabelValues: []string{p.n},
					Value:       boolValue(p.v),
				}
			}

			return &metric.Family{
				Metrics: ms,
			}
		}),
	)
}

func wrapWebsiteFunc(f func(*webhostingv1alpha1.Website) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		website := obj.(*webhostingv1alpha1.Website)
		metricFamily := f(website)

		// populate common labels with values
		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(websiteCommonLabels, m.LabelKeys...)
			m.LabelValues = append([]string{website.Namespace, website.Name}, m.LabelValues...)
		}

		return metricFamily
	}
}
