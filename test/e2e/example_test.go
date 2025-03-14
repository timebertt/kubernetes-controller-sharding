/*
Copyright 2024 Tim Ebert.

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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("Example Shard", Label("example"), Ordered, func() {
	const controllerRingName = "example"

	var (
		controllerRing *shardingv1alpha1.ControllerRing
	)

	BeforeAll(func() {
		controllerRing = &shardingv1alpha1.ControllerRing{ObjectMeta: metav1.ObjectMeta{Name: controllerRingName}}
	})

	Describe("setup", func() {
		It("the Deployment should be healthy", func(ctx SpecContext) {
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "shard", Namespace: metav1.NamespaceDefault}}

			Eventually(ctx, func(g Gomega) {
				g.Expect(Get(deployment)()).To(Succeed())
				g.Expect(deployment.Spec.Replicas).To(PointTo(BeEquivalentTo(3)))
				g.Expect(deployment.Status.AvailableReplicas).To(BeEquivalentTo(3))
			}).Should(Succeed())
		}, SpecTimeout(ShortTimeout))

		It("there should be 3 ready shard leases", func(ctx SpecContext) {
			leaseList := &coordinationv1.LeaseList{}

			Eventually(ctx, func(g Gomega) {
				g.Expect(List(leaseList, client.InNamespace(metav1.NamespaceDefault), client.MatchingLabels{
					shardingv1alpha1.LabelControllerRing: controllerRingName,
				})()).To(Succeed())
				g.Expect(leaseList.Items).To(And(
					HaveLen(3),
					HaveEach(HaveLabelWithValue(shardingv1alpha1.LabelState, "ready")),
				))
			}).Should(Succeed())
		}, SpecTimeout(ShortTimeout))

		It("the ControllerRing should be healthy", func(ctx SpecContext) {
			Eventually(ctx, func(g Gomega) {
				g.Expect(Get(controllerRing)()).To(Succeed())
				g.Expect(controllerRing.Status.Shards).To(BeEquivalentTo(3))
				g.Expect(controllerRing.Status.AvailableShards).To(BeEquivalentTo(3))
				g.Expect(controllerRing.Status.Conditions).To(ConsistOf(
					MatchCondition(
						OfType(shardingv1alpha1.ControllerRingReady),
						WithStatus(metav1.ConditionTrue),
					),
				))
			}).Should(Succeed())
		}, SpecTimeout(ShortTimeout))
	})

	Describe("creating objects", func() {
		var (
			secret *corev1.Secret
			shard  string
		)

		BeforeAll(func() {
			secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-" + test.RandomSuffix(),
				Namespace: metav1.NamespaceDefault,
			}}

			DeferCleanup(func(ctx SpecContext) {
				Eventually(ctx, func() error {
					return testClient.Delete(ctx, secret)
				}).Should(BeNotFoundError())
				log.Info("Deleted object", "secret", client.ObjectKeyFromObject(secret))
			}, NodeTimeout(ShortTimeout))
		})

		It("should assign the main object to a healthy shard", func(ctx SpecContext) {
			shards := getReadyShards(ctx, controllerRingName)

			Expect(testClient.Create(ctx, secret)).To(Succeed())
			log.Info("Created object", "secret", client.ObjectKeyFromObject(secret))

			shard = secret.Labels[controllerRing.LabelShard()]
			Expect(shard).To(BeElementOf(shards))
			Expect(secret).NotTo(HaveLabel(controllerRing.LabelDrain()))
		}, SpecTimeout(ShortTimeout))

		It("should assign the controlled object to the same shard", func(ctx SpecContext) {
			configMap := &corev1.ConfigMap{}
			configMap.Name = "checksums-" + secret.Name
			configMap.Namespace = secret.Namespace

			Eventually(ctx, Object(configMap)).Should(And(
				HaveLabelWithValue(controllerRing.LabelShard(), Equal(shard)),
				Not(HaveLabel(controllerRing.LabelDrain())),
			))
		}, SpecTimeout(ShortTimeout))
	})
})

func getReadyShards(ctx SpecContext, controllerRingName string) []string {
	GinkgoHelper()

	leaseList := &coordinationv1.LeaseList{}
	Eventually(ctx, List(leaseList, client.InNamespace(metav1.NamespaceDefault), client.MatchingLabels{
		shardingv1alpha1.LabelControllerRing: controllerRingName,
		shardingv1alpha1.LabelState:          "ready",
	})).Should(Succeed())

	return toShardNames(leaseList.Items)
}

func toShardNames(leaseList []coordinationv1.Lease) []string {
	out := make([]string, len(leaseList))
	for i, lease := range leaseList {
		out[i] = lease.Name
	}
	return out
}
