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

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

const themeSubsystem = kubeStateMetricsPrefix + "theme_"

var (
	// themeCommonLabels are labels added on each theme metric
	themeCommonLabels = []string{"theme"}
)

type themeFactory struct {
	runtimeClientFactory
}

func newThemeFactory() customresource.RegistryFactory {
	return themeFactory{
		runtimeClientFactory{
			listType: &webhostingv1alpha1.ThemeList{},
		},
	}
}

func (w themeFactory) Name() string {
	return "themes"
}

func (w themeFactory) ExpectedType() interface{} {
	return &webhostingv1alpha1.Theme{}
}

func (w themeFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		themeInfo(),
		themeMetadataGeneration(),
	}
}

func themeInfo() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		themeSubsystem+"info",
		"Information about a Theme.",
		metric.Gauge,
		"",
		wrapThemeFunc(func(t *webhostingv1alpha1.Theme) *metric.Family {
			return &metric.Family{
				Metrics: []*metric.Metric{{
					LabelKeys:   []string{"color", "font_family"},
					LabelValues: []string{t.Spec.Color, t.Spec.FontFamily},
					Value:       1,
				}},
			}
		}),
	)
}

func themeMetadataGeneration() generator.FamilyGenerator {
	return *generator.NewFamilyGenerator(
		themeSubsystem+"metadata_generation",
		"The generation of a Theme.",
		metric.Gauge,
		"",
		wrapThemeFunc(func(t *webhostingv1alpha1.Theme) *metric.Family {
			return &metric.Family{
				Metrics: []*metric.Metric{{
					Value: float64(t.ObjectMeta.Generation),
				}},
			}
		}),
	)
}

func wrapThemeFunc(f func(*webhostingv1alpha1.Theme) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		theme := obj.(*webhostingv1alpha1.Theme)
		metricFamily := f(theme)

		// populate common labels with values
		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(themeCommonLabels, m.LabelKeys...)
			m.LabelValues = append([]string{theme.Name}, m.LabelValues...)
		}

		return metricFamily
	}
}
