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

package exporter

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Exporter is a generic state metrics exporter for a given Kubernetes object kind that can be added to a
// controller-runtime manager.
type Exporter[O client.Object, L client.ObjectList] struct {
	client.Reader

	Namespace, Subsystem string

	// StaticLabelKeys are added to all Metrics of this Exporter.
	StaticLabelKeys []string
	// GenerateStaticLabelValues returns the values for StaticLabelKeys.
	GenerateStaticLabelValues func(O) []string

	Metrics []Metric[O]

	// optional
	ListOptions []client.ListOption
}

// Metric describes a single metric and how to generate it per object.
type Metric[O client.Object] struct {
	Name, Help string
	LabelKeys  []string

	// Generate returns all metric values for the given object.
	// staticLabelValues is the result of Exporter.GenerateStaticLabelValues and should be added as the first label values
	// to all metrics.
	Generate GenerateFunc[O]

	// desc is completed in Exporter.AddToManager.
	desc *prometheus.Desc
}

type GenerateFunc[O client.Object] func(desc *prometheus.Desc, obj O, staticLabelValues []string, ch chan<- prometheus.Metric)

// AddToManager adds this exporter to the given manager and completes the Metrics descriptors.
func (e *Exporter[O, L]) AddToManager(mgr manager.Manager) error {
	if e.Reader == nil {
		e.Reader = mgr.GetCache()
	}

	for i, m := range e.Metrics {
		m.desc = prometheus.NewDesc(
			prometheus.BuildFQName(e.Namespace, e.Subsystem, m.Name),
			m.Help,
			append(e.StaticLabelKeys, m.LabelKeys...),
			nil,
		)
		e.Metrics[i] = m
	}

	return mgr.Add(e)
}

// NeedLeaderElection tells the manager to run the exporter in all instances.
func (e *Exporter[O, L]) NeedLeaderElection() bool {
	return false
}

// Start registers this collector in the controller-runtime metrics registry.
// When Start runs, caches have already been started, so we are ready to export metrics.
func (e *Exporter[O, L]) Start(_ context.Context) error {
	if err := metrics.Registry.Register(e); err != nil {
		return fmt.Errorf("failed to register %s exporter: %w", e.Subsystem, err)
	}

	return nil
}

func (e *Exporter[O, L]) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range e.Metrics {
		ch <- m.desc
	}
}

func (e *Exporter[O, L]) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	list := reflect.New(reflect.TypeOf(*new(L)).Elem()).Interface().(client.ObjectList)
	if err := e.List(ctx, list, e.ListOptions...); err != nil {
		e.handleError(ch, fmt.Errorf("error listing %T: %w", list, err))
		return
	}

	if err := meta.EachListItem(list, func(obj runtime.Object) error {
		o := obj.(O)
		staticLabelValues := e.GenerateStaticLabelValues(o)

		for _, m := range e.Metrics {
			m.Generate(m.desc, o, staticLabelValues, ch)
		}

		return nil
	}); err != nil {
		e.handleError(ch, fmt.Errorf("error iterarting %T: %w", list, err))
		return
	}
}

func (e *Exporter[O, L]) handleError(ch chan<- prometheus.Metric, err error) {
	for _, m := range e.Metrics {
		ch <- prometheus.NewInvalidMetric(m.desc, err)
	}
}

// GenerateStateSet returns a GenerateFunc that emits stateset metrics:
// - it generates one metric per known state
// - the value is 1 if getState returns the state, 0 otherwise
// - if unknownState is given, a metric with this state label is generated with value 1 if no other state has matched
func GenerateStateSet[O client.Object](knownStates []string, unknownState *string, getState func(O) string) GenerateFunc[O] {
	return func(desc *prometheus.Desc, obj O, staticLabelValues []string, ch chan<- prometheus.Metric) {
		actual := getState(obj)

		// generate metrics for all known states
		known := false
		for _, state := range knownStates {
			hasState := actual == state
			if hasState {
				known = true
			}

			ch <- stateSetMetric(desc, state, hasState, staticLabelValues)
		}

		if unknownState != nil {
			// generate a metric for unknown states
			ch <- stateSetMetric(desc, *unknownState, !known, staticLabelValues)
		}
	}
}

func stateSetMetric(desc *prometheus.Desc, state string, value bool, staticLabelValues []string) prometheus.Metric {
	v := 0
	if value {
		v = 1
	}

	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(v), append(staticLabelValues, state)...)
}

// KnownStates converts the given slice of ~string to a slice of strings.
func KnownStates[T ~string](s []T) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = string(s[i])
	}
	return out
}

// KnownStatesStringer converts the given slice of fmt.Stringer to a slice of strings.
func KnownStatesStringer[T fmt.Stringer](s []T) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i].String()
	}
	return out
}
