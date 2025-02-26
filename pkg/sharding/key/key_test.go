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

package key_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/key"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("#FuncForResource", func() {
	var controllerRing *shardingv1alpha1.ControllerRing

	BeforeEach(func() {
		controllerRing = &shardingv1alpha1.ControllerRing{
			Spec: shardingv1alpha1.ControllerRingSpec{
				Resources: []shardingv1alpha1.RingResource{
					{
						GroupResource: metav1.GroupResource{
							Group:    "operator",
							Resource: "foo",
						},
						ControlledResources: []metav1.GroupResource{
							{
								Group:    "operator",
								Resource: "controlled",
							},
							{
								Resource: "foo",
							},
						},
					},
					{
						GroupResource: metav1.GroupResource{
							Resource: "foo",
						},
					},
				},
			},
		}
	})

	It("should return an error if the resource is not part of the ring", func() {
		Expect(FuncForResource(metav1.GroupResource{
			Resource: "bar",
		}, controllerRing)).Error().To(
			MatchError(ContainSubstring("not found")),
		)
	})

	It("should return ForObject if the resource is a main resource of the ring", func() {
		Expect(FuncForResource(metav1.GroupResource{
			Group:    "operator",
			Resource: "foo",
		}, controllerRing)).To(
			BeFunc(ForObject),
		)
	})

	It("should return ForController if the resource is a controlled resource of the ring", func() {
		Expect(FuncForResource(metav1.GroupResource{
			Group:    "operator",
			Resource: "controlled",
		}, controllerRing)).To(
			BeFunc(ForController),
		)
	})

	It("should return ForObject if the resource is a main and controlled resource of the ring", func() {
		Expect(FuncForResource(metav1.GroupResource{
			Resource: "foo",
		}, controllerRing)).To(
			BeFunc(ForObject),
		)
	})
})

var _ = Describe("#ForObject", func() {
	var obj *appsv1.Deployment

	BeforeEach(func() {
		obj = &appsv1.Deployment{}
		obj.GetObjectKind().SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("Deployment"))
		obj.Name = "foo"
		obj.Namespace = "bar"
	})

	It("should return an error if the object has no TypeMeta", func() {
		Expect(ForObject(&appsv1.Deployment{})).Error().To(MatchError("apiVersion and kind must not be empty"))
	})

	It("should return an error if the object has no name", func() {
		obj.Name = ""
		Expect(ForObject(obj)).Error().To(MatchError("name must not be empty"))

		obj.GenerateName = "foo-"
		Expect(ForObject(obj)).Error().To(MatchError(ContainSubstring("generateName is not supported")))
	})

	It("should return the object's hash key", func() {
		Expect(ForObject(obj)).To(Equal("apps/Deployment/bar/foo"))
	})
})

var _ = Describe("#ForController", func() {
	var obj *appsv1.Deployment

	BeforeEach(func() {
		obj = &appsv1.Deployment{}
		obj.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "other/v1",
				Kind:       "Bar",
				Name:       "owner-but-not-controller",
			},
			{
				APIVersion: "operator/v1",
				Kind:       "Foo",
				Name:       "foo",
				Controller: ptr.To(true),
			},
		})
		obj.Namespace = "bar"
	})

	It("should return an empty key if the object has no controller ref", func() {
		Expect(ForController(&appsv1.Deployment{})).To(BeEmpty())

		obj.OwnerReferences[1].Controller = nil
		Expect(ForController(obj)).To(BeEmpty())
	})

	It("should return an error if the controller ref has no apiVersion", func() {
		obj.OwnerReferences[1].APIVersion = ""
		Expect(ForController(obj)).Error().To(MatchError("apiVersion of controller reference must not be empty"))
	})

	It("should return an error if the controller ref has no kind", func() {
		obj.OwnerReferences[1].Kind = ""
		Expect(ForController(obj)).Error().To(MatchError("kind of controller reference must not be empty"))
	})

	It("should return an error if the controller ref has no name", func() {
		obj.OwnerReferences[1].Name = ""
		Expect(ForController(obj)).Error().To(MatchError("name of controller reference must not be empty"))
	})

	It("should return an error if the controller ref has an invalid apiVersion", func() {
		obj.OwnerReferences[1].APIVersion = "foo/bar/v1"
		Expect(ForController(obj)).Error().To(MatchError(ContainSubstring("invalid apiVersion of controller reference")))
	})

	It("should return the controller's hash key", func() {
		Expect(ForController(obj)).To(Equal("operator/Foo/bar/foo"))
	})
})
