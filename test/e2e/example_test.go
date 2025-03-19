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
	"context"
	"fmt"
	"maps"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

const (
	checksumControllerName = "checksum-controller"
	namePrefixChecksums    = "checksums-"

	objectCount = 100
)

var _ = Describe("Example Controller", Label(checksumControllerName), func() {
	Describe("setup", Ordered, func() {
		It("the sharder Deployment should be healthy", func(ctx SpecContext) {
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "sharder", Namespace: shardingv1alpha1.NamespaceSystem}}
			Eventually(ctx, Object(deployment)).Should(And(
				HaveField("Spec.Replicas", HaveValue(BeEquivalentTo(2))),
				HaveField("Status.AvailableReplicas", BeEquivalentTo(2)),
			))
		}, SpecTimeout(ShortTimeout))

		It("the controller Deployment should be healthy", func(ctx SpecContext) {
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: checksumControllerName, Namespace: namespace.Name}}
			Eventually(ctx, Object(deployment)).Should(And(
				HaveField("Spec.Replicas", HaveValue(BeEquivalentTo(3))),
				HaveField("Status.AvailableReplicas", BeEquivalentTo(3)),
			))
		}, SpecTimeout(ShortTimeout))

		itControllerRingShouldBeReady()
		itShouldGetReadyShards(3)

		It("there should not be any shard leases other than the 3 ready leases", func(ctx SpecContext) {
			leaseList := &coordinationv1.LeaseList{}
			Eventually(ctx, ObjectList(leaseList, client.InNamespace(namespace.Name),
				client.MatchingLabels{shardingv1alpha1.LabelControllerRing: controllerRing.Name},
			)).Should(HaveNames(shards...))
		}, SpecTimeout(ShortTimeout))

		It("should create the MutatingWebhookConfiguration", func(ctx SpecContext) {
			webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "controllerring-" + controllerRing.Name},
			}
			Eventually(ctx, Object(webhookConfig)).Should(And(
				HaveLabelWithValue(shardingv1alpha1.LabelControllerRing, controllerRing.Name),
				HaveField("Webhooks", ConsistOf(And(
					HaveField("NamespaceSelector", Equal(&metav1.LabelSelector{
						MatchLabels: map[string]string{corev1.LabelMetadataName: namespace.Name},
					})),
					HaveField("ObjectSelector", Equal(&metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      controllerRing.LabelShard(),
							Operator: metav1.LabelSelectorOpDoesNotExist,
						}},
					})),
					HaveField("Rules", ConsistOf(
						HaveField("Resources", ConsistOf("secrets")),
						HaveField("Resources", ConsistOf("configmaps")),
					)),
				))),
			))
		}, SpecTimeout(ShortTimeout))
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

		itControllerRingShouldBeReady()
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

		objectLabels := itShouldCreateObjects()

		It("should assign objects to all shards", func(ctx SpecContext) {
			var usedShards sets.Set[string]
			Eventually(ctx, func(g Gomega) {
				secretsList := &corev1.SecretList{}
				g.Expect(ObjectList(secretsList, client.InNamespace(namespace.Name), client.MatchingLabels(objectLabels))()).To(HaveField("Items", HaveLen(objectCount)))

				configMapList := &corev1.ConfigMapList{}
				g.Expect(ObjectList(configMapList, client.InNamespace(namespace.Name), client.MatchingLabels(objectLabels))()).To(HaveField("Items", HaveLen(objectCount)))
				configMaps := toMapOfConfigMap(configMapList.Items)

				usedShards = sets.New[string]()
				for _, secret := range secretsList.Items {
					g.Expect(secret).To(HaveLabelWithValue(controllerRing.LabelShard(), BeElementOf(shards)))
					g.Expect(secret).NotTo(HaveLabel(controllerRing.LabelDrain()))

					configMap := configMaps[namePrefixChecksums+secret.Name]
					g.Expect(configMap).NotTo(BeNil(), "there should be a checksum ConfigMap for Secret %s", secret.Name)

					g.Expect(configMap).NotTo(HaveLabel(controllerRing.LabelDrain()))
					g.Expect(configMap.Labels[controllerRing.LabelShard()]).To(Equal(secret.Labels[controllerRing.LabelShard()]),
						"ConfigMap %s should be assigned to the same shard as the owning Secret", configMap.Name)
					usedShards.Insert(secret.Labels[controllerRing.LabelShard()])
				}
			}).Should(Succeed(), "ConfigMaps should be assigned to the same shard as the owning Secrets")

			Expect(usedShards.UnsortedList()).To(ConsistOf(shards), "should use all available shards")
		}, SpecTimeout(MediumTimeout))
	})

	describeScaleController("adding a shard", 4)

	describeScaleController("removing a shard", 2)

	Describe("graceful shard termination", Ordered, func() {
		lease := &coordinationv1.Lease{}

		BeforeAll(func() {
			*lease = *newLease(60)
		})

		itControllerRingShouldBeReady()

		itShouldCreateShardLease(lease)
		itShardShouldHaveState(lease, leases.Ready)
		itControllerRingShouldHaveAvailableShard(4)

		It("should release the shard lease", func(ctx SpecContext) {
			patch := client.MergeFrom(lease.DeepCopy())
			lease.Spec.HolderIdentity = nil
			Expect(testClient.Patch(ctx, lease, patch)).To(Succeed())
		}, SpecTimeout(ShortTimeout))

		itShardShouldHaveState(lease, leases.Dead)
		itControllerRingShouldHaveAvailableShard(3)
	})

	Describe("shard failure detection", Ordered, func() {
		lease := &coordinationv1.Lease{}

		BeforeAll(func() {
			*lease = *newLease(10)
		})

		itControllerRingShouldBeReady()

		itShouldCreateShardLease(lease)
		itShardShouldHaveState(lease, leases.Ready)
		itControllerRingShouldHaveAvailableShard(4)

		It("should transition the shard lease to state expired", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(
				HaveLabelWithValue(shardingv1alpha1.LabelState, leases.Expired.String()),
			)
		}, SpecTimeout(15*time.Second))

		It("should acquire the shard lease", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(And(
				HaveField("Spec.HolderIdentity", HaveValue(Equal(shardingv1alpha1.IdentityShardLeaseController))),
				HaveLabelWithValue(shardingv1alpha1.LabelState, leases.Dead.String()),
			), "lease should be acquired by sharder")
		}, SpecTimeout(15*time.Second))

		itControllerRingShouldHaveAvailableShard(3)
	})
})

func describeScaleController(text string, replicas int32) {
	Describe(text, Ordered, func() {
		itControllerRingShouldBeReady()
		itShouldGetReadyShards(3)

		objectLabels := itShouldCreateObjects()
		itObjectsShouldBeAssignedToShards(objectLabels)

		itShouldScaleTheController(replicas)
		itControllerRingShouldHaveAvailableShard(replicas)
		itShouldGetReadyShards(int(replicas))

		itObjectsShouldBeAssignedToShards(objectLabels)
	})
}

func itControllerRingShouldBeReady() {
	GinkgoHelper()

	It("the ControllerRing should be ready", func(ctx SpecContext) {
		Eventually(ctx, Object(controllerRing)).Should(And(
			HaveField("Status.Shards", BeEquivalentTo(3)),
			HaveField("Status.AvailableShards", BeEquivalentTo(3)),
			HaveField("Status.Conditions", ConsistOf(
				MatchCondition(
					OfType(shardingv1alpha1.ControllerRingReady),
					WithStatus(metav1.ConditionTrue),
				),
			)),
		))
	}, SpecTimeout(ShortTimeout))
}

func itControllerRingShouldHaveAvailableShard(expectedAvailableShards int32) {
	GinkgoHelper()

	It(fmt.Sprintf("the ControllerRing should be ready and should have %d available shards", expectedAvailableShards), func(ctx SpecContext) {
		Eventually(ctx, Object(controllerRing)).Should(And(
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
			shardingv1alpha1.LabelState:          leases.Ready.String(),
		})).Should(HaveField("Items", HaveLen(expectedCount)))

		shards = make([]string, len(leaseList.Items))
		for i, lease := range leaseList.Items {
			shards[i] = lease.Name
		}
	}, SpecTimeout(ShortTimeout))
}

func itShouldCreateShardLease(lease *coordinationv1.Lease) {
	GinkgoHelper()

	It("should create a new shard lease", func(ctx SpecContext) {
		microNow := metav1.NewMicroTime(time.Now())
		lease.Spec.AcquireTime = ptr.To(microNow)
		lease.Spec.RenewTime = ptr.To(microNow)

		Expect(testClient.Create(ctx, lease)).To(Succeed())
	}, SpecTimeout(ShortTimeout))
}

func itShardShouldHaveState(lease *coordinationv1.Lease, state leases.ShardState) {
	GinkgoHelper()

	It("the shard should have state "+state.String(), func(ctx SpecContext) {
		Eventually(ctx, Object(lease)).Should(
			HaveLabelWithValue(shardingv1alpha1.LabelState, state.String()),
		)
	}, SpecTimeout(ShortTimeout))
}

func itShouldCreateObjects() map[string]string {
	GinkgoHelper()

	labels := map[string]string{testID + "-objects": testRunID + test.RandomSuffix()}

	It("should create many objects", func(ctx SpecContext) {
		for i := 0; i < objectCount; i++ {
			newSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "foo-" + strconv.Itoa(i),
				Labels:    maps.Clone(labels),
			}}
			Expect(testClient.Create(ctx, newSecret)).To(Succeed(), "should create secret %s", newSecret.Name)
		}
	}, SpecTimeout(MediumTimeout))

	return labels
}

func itShouldScaleTheController(replicas int32) {
	GinkgoHelper()

	It(fmt.Sprintf("should scale the controller to %d replicas", replicas), func(ctx SpecContext) {
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: checksumControllerName, Namespace: namespace.Name}}

		patch := client.MergeFrom(&autoscalingv1.Scale{})
		scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: replicas}}
		Expect(testClient.SubResource("scale").Patch(ctx, deployment, patch, client.WithSubResourceBody(scale), &client.SubResourcePatchOptions{})).To(Succeed())
	}, NodeTimeout(ShortTimeout))
}

func itObjectsShouldBeAssignedToShards(labels map[string]string) {
	GinkgoHelper()

	It("should assign the Secrets to the available shards", func(ctx SpecContext) {
		eventuallyObjectsShouldBeAssignedToShards(ctx, &corev1.SecretList{}, labels)
	}, NodeTimeout(MediumTimeout))

	It("should assign the ConfigMaps to the available shards", func(ctx SpecContext) {
		eventuallyObjectsShouldBeAssignedToShards(ctx, &corev1.SecretList{}, labels)
	}, NodeTimeout(MediumTimeout))
}

func eventuallyObjectsShouldBeAssignedToShards(ctx context.Context, list client.ObjectList, labels map[string]string) {
	GinkgoHelper()

	Eventually(ctx, func(g Gomega) {
		g.Expect(ObjectList(list, client.InNamespace(namespace.Name), client.MatchingLabels(labels))()).To(HaveField("Items", HaveLen(objectCount)))

		usedShards := sets.New[string]()
		Expect(meta.EachListItem(list, func(obj runtime.Object) error {
			g.Expect(obj).To(HaveLabelWithValue(controllerRing.LabelShard(), Not(BeEmpty())), "object %T %s should be assigned")
			g.Expect(obj).NotTo(HaveLabel(controllerRing.LabelDrain()), "object %T %s should not have drain label")

			usedShards.Insert(obj.(client.Object).GetLabels()[controllerRing.LabelShard()])
			return nil
		})).To(Succeed())

		g.Expect(usedShards.UnsortedList()).To(ConsistOf(shards), "should use all available shards")
	}).Should(Succeed())
}

func newLease(leaseDurationSeconds int32) *coordinationv1.Lease {
	name := "test-" + test.RandomSuffix()

	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
			Labels: map[string]string{
				shardingv1alpha1.LabelControllerRing: controllerRing.Name,
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(name),
			LeaseDurationSeconds: ptr.To[int32](leaseDurationSeconds),
		},
	}
}

func toMapOfConfigMap(configMaps []corev1.ConfigMap) map[string]*corev1.ConfigMap {
	out := make(map[string]*corev1.ConfigMap, len(configMaps))
	for _, configMap := range configMaps {
		out[configMap.Name] = &configMap
	}
	return out
}
