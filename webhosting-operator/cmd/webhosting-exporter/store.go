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
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/options"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/webhosting/v1alpha1"
)

const kubeStateMetricsPrefix = "kube_"

var (
	scheme = runtime.NewScheme()

	exporterResources = options.ResourceSet{}
	customFactories   = []customresource.RegistryFactory{
		newWebsiteFactory(),
		newThemeFactory(),
	}
)

func init() {
	utilruntime.Must(webhostingv1alpha1.AddToScheme(scheme))

	registerResources()
}

func registerResources() {
	for _, f := range customFactories {
		exporterResources[f.Name()] = struct{}{}
	}
}

// runtimeClientFactory implements parts of customresource.RegistryFactory for reuse.
type runtimeClientFactory struct {
	listType client.ObjectList
}

func (r runtimeClientFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return client.NewWithWatch(cfg, client.Options{
		Scheme: scheme,
	})
}

func (r runtimeClientFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	c := customResourceClient.(client.WithWatch)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			list := r.listType.DeepCopyObject().(client.ObjectList)
			opts.FieldSelector = fieldSelector
			err := c.List(context.TODO(), list, &client.ListOptions{Raw: &opts, Namespace: ns})
			return list, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			list := r.listType.DeepCopyObject().(client.ObjectList)
			opts.FieldSelector = fieldSelector
			return c.Watch(context.TODO(), list, &client.ListOptions{Raw: &opts, Namespace: ns})
		},
	}
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
