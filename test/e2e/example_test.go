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

const namePrefixChecksums = "checksums-"

var _ = Describe("Example Controller", Label("checksum-controller"), func() {
	Describe("setup", Ordered, func() {
		It("the Deployment should be healthy", func(ctx SpecContext) {
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "checksum-controller", Namespace: namespace.Name}}
			Eventually(ctx, Object(deployment)).Should(And(
				HaveField("Spec.Replicas", HaveValue(BeEquivalentTo(3))),
				HaveField("Status.AvailableReplicas", BeEquivalentTo(3)),
			))
		}, SpecTimeout(ShortTimeout))

		It("there should be 3 ready shard leases", func(ctx SpecContext) {
			leaseList := &coordinationv1.LeaseList{}
			Eventually(ctx, ObjectList(leaseList, client.InNamespace(namespace.Name), client.MatchingLabelsSelector{
				Selector: controllerRing.LeaseSelector(),
			})).Should(HaveField("Items", And(
				HaveLen(3),
				HaveEach(HaveLabelWithValue(shardingv1alpha1.LabelState, "ready")),
			)))
		}, SpecTimeout(ShortTimeout))

		itControllerRingShouldBeReady(3, 3)
		itShouldGetReadyShards(3)
	})

	Describe("creating objects", Ordered, func() {
		var (
			secret *corev1.Secret
			shard  string
		)

		BeforeAll(func() {
			secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-" + test.RandomSuffix(),
				Namespace: namespace.Name,
			}}
		})

		itControllerRingShouldBeReady(3, 3)
		itShouldGetReadyShards(3)

		It("should assign the main object to a healthy shard", func(ctx SpecContext) {
			// Verify that the sharder successfully injects the shard label.
			// The webhook has failurePolicy=Ignore, so we might need to delete the secret and try again until the injection
			// succeeds.
			newSecret := secret.DeepCopy()

			Eventually(ctx, func(g Gomega) *corev1.Secret {
				// if the secret has already been created, reset it and try again
				newSecret.DeepCopyInto(secret)

				g.Expect(testClient.Create(ctx, secret)).To(Succeed())
				return secret
			}).Should(And(
				HaveLabelWithValue(controllerRing.LabelShard(), BeElementOf(shards)),
				Not(HaveLabel(controllerRing.LabelDrain())),
			))
			shard = secret.Labels[controllerRing.LabelShard()]

			log.Info("Created object", "secret", client.ObjectKeyFromObject(secret), "shard", shard)
		}, SpecTimeout(MediumTimeout))

		It("should assign the controlled object to the same shard", func(ctx SpecContext) {
			configMap := &corev1.ConfigMap{}
			configMap.Name = namePrefixChecksums + secret.Name
			configMap.Namespace = secret.Namespace

			Eventually(ctx, Object(configMap)).Should(And(
				HaveLabelWithValue(controllerRing.LabelShard(), Equal(shard)),
				Not(HaveLabel(controllerRing.LabelDrain())),
			))
		}, SpecTimeout(MediumTimeout))
	})
})

func itControllerRingShouldBeReady(expectedShards, expectedAvailableShards int) {
	GinkgoHelper()

	It("the ControllerRing should be ready", func(ctx SpecContext) {
		Eventually(ctx, Object(controllerRing)).Should(And(
			HaveField("Status.Shards", BeEquivalentTo(expectedShards)),
			HaveField("Status.AvailableShards", BeEquivalentTo(expectedAvailableShards)),
			HaveField("Status.Conditions", ConsistOf(
				MatchCondition(
					OfType(shardingv1alpha1.ControllerRingReady),
					WithStatus(metav1.ConditionTrue),
				),
			)),
		))
	}, SpecTimeout(ShortTimeout))
}

var shards []string

func itShouldGetReadyShards(expectedCount int) {
	GinkgoHelper()

	It("should get the ready shards", func(ctx SpecContext) {
		leaseList := &coordinationv1.LeaseList{}
		Eventually(ctx, ObjectList(leaseList, client.InNamespace(namespace.Name), client.MatchingLabels{
			shardingv1alpha1.LabelControllerRing: controllerRing.Name,
			shardingv1alpha1.LabelState:          "ready",
		})).Should(HaveField("Items", HaveLen(expectedCount)))

		shards = toShardNames(leaseList.Items)
	}, SpecTimeout(MediumTimeout))
}

func toShardNames(leaseList []coordinationv1.Lease) []string {
	out := make([]string, len(leaseList))
	for i, lease := range leaseList {
		out[i] = lease.Name
	}
	return out
}
