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

package sharding

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KeyFunc func(schema.GroupKind, client.Object) string

func DefaultKeyFunc(gk schema.GroupKind, obj client.Object) string {
	// can't rely on obj.GetObjectKind() as corresponding fields are cleared in rest client
	return defaultKeyFunc(gk, obj.GetNamespace(), obj.GetName())
}

func defaultKeyFunc(gk schema.GroupKind, namespace, name string) string {
	return gk.String() + "/" + namespace + "/" + name
}

func KeyForController(_ schema.GroupKind, obj client.Object) string {
	ref := metav1.GetControllerOf(obj)
	if ref == nil {
		return ""
	}

	gvk := schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind)
	return defaultKeyFunc(gvk.GroupKind(), obj.GetNamespace(), ref.Name)
}
