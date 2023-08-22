/*
Copyright 2023 Tim Ebert.

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

package generator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GenerateOption interface {
	ApplyToOptions(options *GenerateOptions)
}

type GenerateOptions struct {
	Labels         map[string]string
	OwnerReference *metav1.OwnerReference
}

func (o *GenerateOptions) ApplyOptions(opts ...GenerateOption) *GenerateOptions {
	for _, opt := range opts {
		opt.ApplyToOptions(o)
	}
	return o
}

func (o *GenerateOptions) ApplyToOptions(options *GenerateOptions) {
	if len(o.Labels) > 0 {
		if options.Labels == nil {
			options.Labels = make(map[string]string, len(o.Labels))
		}

		for k, v := range o.Labels {
			options.Labels[k] = v
		}
	}

	if ownerRef := o.OwnerReference; ownerRef != nil {
		options.OwnerReference = ownerRef.DeepCopy()
	}
}

func (o *GenerateOptions) ApplyToObject(obj *metav1.ObjectMeta) {
	for k, v := range o.Labels {
		metav1.SetMetaDataLabel(obj, k, v)
	}

	if ownerRef := o.OwnerReference; ownerRef != nil {
		obj.OwnerReferences = append(obj.OwnerReferences, *ownerRef)
	}
}

type GenerateOptionFunc func(options *GenerateOptions)

func (f GenerateOptionFunc) ApplyToOptions(options *GenerateOptions) {
	f(options)
}

func WithLabels(labels map[string]string) GenerateOption {
	return GenerateOptionFunc(func(options *GenerateOptions) {
		if options.Labels == nil {
			options.Labels = make(map[string]string, len(labels))
		}

		for k, v := range labels {
			options.Labels[k] = v
		}
	})
}

func WithOwnerReference(ownerRef *metav1.OwnerReference) GenerateOption {
	return GenerateOptionFunc(func(options *GenerateOptions) {
		options.OwnerReference = ownerRef.DeepCopy()
	})
}
