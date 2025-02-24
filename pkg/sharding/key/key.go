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

package key

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// FuncForResource returns the key function that maps the given resource or its controller depending on whether
// the resource is listed as a resource or controlled resource in the given ring.
func FuncForResource(gr metav1.GroupResource, ring *shardingv1alpha1.ControllerRing) (Func, error) {
	ringResources := sets.New[metav1.GroupResource]()
	controlledResources := sets.New[metav1.GroupResource]()

	for _, ringResource := range ring.Spec.Resources {
		ringResources.Insert(ringResource.GroupResource)

		for _, controlledResource := range ringResource.ControlledResources {
			controlledResources.Insert(controlledResource)
		}
	}

	switch {
	case ringResources.Has(gr):
		return ForObject, nil
	case controlledResources.Has(gr):
		return ForController, nil
	}

	return nil, fmt.Errorf("object's resource %q was not found in ControllerRing", gr.String())
}

// Func maps objects to hash keys.
// It returns an error if the prerequisites for sharding the given object are not fulfilled.
// If the returned key is empty, the object should not be assigned.
type Func func(client.Object) (string, error)

// ForObject returns a ring key for the given object itself.
// It needs the TypeMeta (GVK) to be set, which is not set on objects after decoding by default.
func ForObject(obj client.Object) (string, error) {
	// We can't use the object's UID, as it is unset during admission for CREATE requests.
	// Instead, we need to calculate a unique ID ourselves. The ID has this pattern (see forMetadata):
	//  group/version/kind/namespace/name
	// With this, different object instances with the same name will use the same hash key, which sounds acceptable.
	// We can only use fields that are also present in owner references as we need to assign owners and ownees to the same
	// shard. E.g., we can't use generateName.

	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return "", fmt.Errorf("apiVersion and kind must not be empty")
	}

	if obj.GetName() == "" {
		if obj.GetGenerateName() != "" {
			// If generateName is used, name is unset during admission for CREATE requests.
			// We can't support assigning such objects during admission because we will not be able to calculate a unique
			// object ID that we can also reconstruct later on for owned objects just by looking at the object itself.
			// We could use a cache lookup though, but this would restrict scalability of the sharding solution again.
			// Generally, this tradeoff seems acceptable, as generateName is mostly used on owned objects, but rarely the
			// owner itself. In such case, ForController will be used instead, which doesn't care about the object's own
			// name but only that of the owner.
			// If generateName is used nevertheless, respond with a proper error.
			// We could assign the object after creation, however we can't use a watch cache because of the mentioned
			// scalability limitations. A possible solution could only do some optimistic delayed enqueuing.
			return "", fmt.Errorf("generateName is not supported on ring resources that are not controlled by another resource")
		}

		return "", fmt.Errorf("name must not be empty")
	}

	// Namespace can be empty for cluster-scoped resources. Only check the name field as an optimistic check for
	// preventing wrong usage of the function.
	return forMetadata(gvk.Group, gvk.Kind, obj.GetNamespace(), obj.GetName()), nil
}

// ForController returns a ring key for the controller of the given object.
// It returns an empty key if the object doesn't have an ownerReference with controller=true".
func ForController(obj client.Object) (string, error) {
	ref := metav1.GetControllerOf(obj)
	if ref == nil {
		return "", nil
	}

	if ref.APIVersion == "" {
		return "", fmt.Errorf("apiVersion of controller reference must not be empty")
	}
	if ref.Kind == "" {
		return "", fmt.Errorf("kind of controller reference must not be empty")
	}
	if ref.Name == "" {
		return "", fmt.Errorf("name of controller reference must not be empty")
	}

	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return "", fmt.Errorf("invalid apiVersion of controller reference: %w", err)
	}

	// Namespace can be empty for cluster-scoped resources. Only check the other fields as an optimistic check for
	// preventing wrong usage of the function.
	return forMetadata(gv.Group, ref.Kind, obj.GetNamespace(), ref.Name), nil
}

func forMetadata(group, kind, namespace, name string) string {
	return group + "/" + kind + "/" + namespace + "/" + name
}
