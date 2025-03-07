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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
	. "github.com/timebertt/kubernetes-controller-sharding/test/integration/shard/controller"
)

var obj *corev1.ConfigMap

var _ = Describe("Shard controller", func() {
	BeforeEach(func() {
		obj = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
				Namespace:    testRunID,
				Labels: map[string]string{
					shardLabel: shardName,
					LabelKey:   LabelValuePending,
				},
			},
		}
	}, OncePerOrdered)

	JustBeforeEach(func(ctx SpecContext) {
		Expect(testClient.Create(ctx, obj)).To(Succeed())
		log.Info("Created ConfigMap for test", "configMapName", obj.Name)
	}, NodeTimeout(time.Minute), OncePerOrdered)

	When("the object is assigned on Create event", func() {
		itShouldReconcile()
		itShouldDrain()

		When("the predicate does not match the object", func() {
			BeforeEach(ignoreObject, OncePerOrdered)
			itShouldNotReconcile()
			itShouldDrain()
		})

		When("the object should be drained on Create event", Ordered, func() {
			BeforeAll(drainObject)
			itShouldRemoveShardingLabels()
			itShouldNotReconcile()
		})
	})

	When("the object is assigned to a different shard on Create event", func() {
		BeforeEach(assignOtherShard, OncePerOrdered)
		itShouldNotReconcile()
		itShouldNotDrain()

		When("the object gets assigned", Ordered, func() {
			itShouldNotReconcile()

			It("assign object", func(ctx SpecContext) {
				Eventually(ctx, Update(obj, assignThisShard)).Should(Succeed())
			}, SpecTimeout(time.Minute))

			itShouldReconcile()
		})
	})
})

func itShouldReconcile() {
	GinkgoHelper()

	It("should reconcile the object", func(ctx SpecContext) {
		Eventually(ctx, Object(obj)).Should(haveStatus(LabelValueDone))
	}, SpecTimeout(time.Minute))
}

func itShouldNotReconcile() {
	GinkgoHelper()

	It("should not reconcile the object", func(ctx SpecContext) {
		Consistently(ctx, Object(obj)).Should(haveStatus(obj.Labels[LabelKey]))
	}, SpecTimeout(time.Minute))
}

func itShouldDrain() {
	GinkgoHelper()

	Describe("should handle the drain operation", Ordered, func() {
		itAddDrainLabel()
		itShouldRemoveShardingLabels()
		itShouldNotReconcile()
	})
}

func itShouldNotDrain() {
	GinkgoHelper()

	Context("should not handle drain operation", Ordered, func() {
		itAddDrainLabel()

		It("should not remove shard and drain labels", func(ctx SpecContext) {
			Consistently(ctx, Object(obj)).Should(And(
				HaveLabel(shardLabel),
				HaveLabel(drainLabel),
			))
		}, SpecTimeout(time.Minute))
	})
}

func itAddDrainLabel() {
	GinkgoHelper()

	It("add the drain label", func(ctx SpecContext) {
		Eventually(ctx, Update(obj, drainObject)).Should(Succeed())
	}, SpecTimeout(time.Minute))
}

func itShouldRemoveShardingLabels() {
	GinkgoHelper()

	It("should remove shard and drain labels", func(ctx SpecContext) {
		Eventually(ctx, Object(obj)).Should(And(
			Not(HaveLabel(shardLabel)),
			Not(HaveLabel(drainLabel)),
		))
	}, SpecTimeout(time.Minute))
}

func haveStatus(status string) gomegatypes.GomegaMatcher {
	return HaveLabelWithValue(LabelKey, status)
}

func ignoreObject() {
	obj.Labels[LabelKey] = LabelValueIgnore
}

func assignThisShard() {
	obj.Labels[shardLabel] = shardName
}

func assignOtherShard() {
	obj.Labels[shardLabel] = "other"
}

func drainObject() {
	obj.Labels[drainLabel] = "true"
}
