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

package controllerring_test

import (
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("ControllerRing controller", func() {
	var (
		controllerRing *shardingv1alpha1.ControllerRing
	)

	BeforeEach(func(ctx SpecContext) {
		controllerRing = &shardingv1alpha1.ControllerRing{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: testRunID + "-",
				Labels:       maps.Clone(testRunLabels),
			},
			Spec: shardingv1alpha1.ControllerRingSpec{
				Resources: []shardingv1alpha1.RingResource{
					{
						GroupResource: metav1.GroupResource{Group: "apps", Resource: "deployments"},
					},
				},
				NamespaceSelector: nil,
			},
		}

		Expect(testClient.Create(ctx, controllerRing)).To(Succeed())
		log.Info("Created ControllerRing for test", "controllerRingName", controllerRing.Name)

		DeferCleanup(func(ctx SpecContext) {
			Expect(testClient.Delete(ctx, controllerRing)).To(Or(Succeed(), BeNotFoundError()))
		}, NodeTimeout(time.Minute))
	}, NodeTimeout(time.Minute), OncePerOrdered)

	It("should observe the generation", func(ctx SpecContext) {
		Eventually(ctx, Object(controllerRing)).Should(
			HaveField("Status.ObservedGeneration", Equal(controllerRing.Generation)),
		)
	}, SpecTimeout(time.Minute))

	It("should report readiness", func(ctx SpecContext) {
		Eventually(ctx, Object(controllerRing)).Should(
			HaveField("Status.Conditions", ConsistOf(
				MatchCondition(
					OfType(shardingv1alpha1.ControllerRingReady),
					WithStatus(metav1.ConditionTrue),
				),
			)),
		)
	}, SpecTimeout(time.Minute))

	It("should apply the sharder webhook", func(ctx SpecContext) {
		webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "controllerring-" + controllerRing.Name},
		}
		Eventually(ctx, Object(webhookConfig)).Should(And(
			HaveField("ObjectMeta.OwnerReferences", ConsistOf(And(
				HaveField("Kind", Equal("ControllerRing")),
				HaveField("Name", Equal(controllerRing.Name)),
			))),
			HaveField("Webhooks", ConsistOf(And(
				HaveField("ClientConfig.Service.Path", HaveValue(Equal("/webhooks/sharder/controllerring/"+controllerRing.Name))),
				HaveField("Rules", ConsistOf(And(
					HaveField("APIGroups", ConsistOf("apps")),
					HaveField("Resources", ConsistOf("deployments")),
				))),
			))),
		))
	}, SpecTimeout(time.Minute))

	Describe("should reflect the shard leases in the status", Ordered, func() {
		var lease *coordinationv1.Lease

		It("Create available shard lease", func(ctx SpecContext) {
			lease = newLease(controllerRing.Name)
			Expect(testClient.Create(ctx, lease)).To(Succeed())

			Eventually(ctx, Object(controllerRing)).Should(haveStatusShards(1, 1))
		}, SpecTimeout(time.Minute))

		It("Create orphaned shard lease", func(ctx SpecContext) {
			lease = newLease(controllerRing.Name)
			lease.Spec.HolderIdentity = nil
			Expect(testClient.Create(ctx, lease)).To(Succeed())

			Eventually(ctx, Object(controllerRing)).Should(haveStatusShards(1, 2))
		}, SpecTimeout(time.Minute))

		It("Make lease healthy", func(ctx SpecContext) {
			Eventually(ctx, Update(lease, func() {
				lease.Spec.HolderIdentity = ptr.To(lease.Name)
			})).Should(Succeed())

			Eventually(ctx, Object(controllerRing)).Should(haveStatusShards(2, 2))
		}, SpecTimeout(time.Minute))

		It("Make lease unhealthy", func(ctx SpecContext) {
			Eventually(ctx, Update(lease, func() {
				lease.Spec.HolderIdentity = nil
			})).Should(Succeed())

			Eventually(ctx, Object(controllerRing)).Should(haveStatusShards(1, 2))
		}, SpecTimeout(time.Minute))

		It("Delete unhealthy lease", func(ctx SpecContext) {
			Expect(testClient.Delete(ctx, lease)).To(Succeed())

			Eventually(ctx, Object(controllerRing)).Should(haveStatusShards(1, 1))
		}, SpecTimeout(time.Minute))
	})
})

func newLease(controllerRingName string) *coordinationv1.Lease {
	name := testRunID + "-" + test.RandomSuffix()

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testRunID,
			Labels:    maps.Clone(testRunLabels),
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(name),
			LeaseDurationSeconds: ptr.To[int32](10),
			AcquireTime:          ptr.To(metav1.NewMicroTime(clock.Now().Add(-5 * time.Minute))),
			RenewTime:            ptr.To(metav1.NewMicroTime(clock.Now().Add(-2 * time.Second))),
		},
	}
	metav1.SetMetaDataLabel(&lease.ObjectMeta, shardingv1alpha1.LabelControllerRing, controllerRingName)

	return lease
}

func haveStatusShards(availableShards, shards int32) gomegatypes.GomegaMatcher {
	return And(
		HaveField("Status.AvailableShards", Equal(availableShards)),
		HaveField("Status.Shards", Equal(shards)),
	)
}
