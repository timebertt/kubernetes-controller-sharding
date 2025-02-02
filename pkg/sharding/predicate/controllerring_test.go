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

package predicate_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/predicate"
)

var _ = Describe("ControllerRing", func() {
	var (
		p predicate.Predicate

		obj, objOld *shardingv1alpha1.ControllerRing
	)

	BeforeEach(func() {
		obj = &shardingv1alpha1.ControllerRing{
			ObjectMeta: metav1.ObjectMeta{
				Generation: 1,
			},
		}
		objOld = obj.DeepCopy()
	})

	Describe("#ControllerRingCreatedOrUpdated", func() {
		BeforeEach(func() {
			p = ControllerRingCreatedOrUpdated()
		})

		It("should react on create events", func() {
			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
		})

		It("should react on spec updates", func() {
			obj.Generation++
			obj.Spec.Resources = append(obj.Spec.Resources, shardingv1alpha1.RingResource{
				GroupResource: metav1.GroupResource{Resource: "pods"},
			})
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should ignore status updates", func() {
			obj.Status.AvailableShards++
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
		})

		It("should ignore delete events", func() {
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
		})
	})
})
