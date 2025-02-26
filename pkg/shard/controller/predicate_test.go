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

package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/controller"
)

var _ = Describe("#Predicate", func() {
	var (
		controllerRingName string
		shardName          string

		mainPredicate, p predicate.Predicate

		obj, objOld *corev1.Pod
	)

	BeforeEach(func() {
		controllerRingName = "operator"
		shardName = "operator-0"

		obj = &corev1.Pod{}
	})

	JustBeforeEach(func() {
		p = Predicate(controllerRingName, shardName, mainPredicate)
	})

	When("main predicate returns false", func() {
		BeforeEach(func() {
			mainPredicate = predicate.NewPredicateFuncs(func(client.Object) bool {
				return false
			})
		})

		It("should handle drained objects", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "drain.alpha.sharding.timebertt.dev/"+controllerRingName, "true")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
		})

		It("should handle assigned and drained objects", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "shard.alpha.sharding.timebertt.dev/"+controllerRingName, shardName)
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "drain.alpha.sharding.timebertt.dev/"+controllerRingName, "true")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
		})

		It("should not handle assigned objects", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "shard.alpha.sharding.timebertt.dev/"+controllerRingName, shardName)
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
			Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeFalse())
		})
	})

	When("main predicate returns true", func() {
		BeforeEach(func() {
			mainPredicate = predicate.NewPredicateFuncs(func(client.Object) bool {
				return true
			})
		})

		It("should handle drained objects", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "drain.alpha.sharding.timebertt.dev/"+controllerRingName, "true")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
		})

		It("should handle assigned objects", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "shard.alpha.sharding.timebertt.dev/"+controllerRingName, shardName)
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
		})

		It("should handle assigned and drained objects", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "shard.alpha.sharding.timebertt.dev/"+controllerRingName, shardName)
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "drain.alpha.sharding.timebertt.dev/"+controllerRingName, "true")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
			Expect(p.Generic(event.GenericEvent{Object: obj})).To(BeTrue())
		})
	})
})
