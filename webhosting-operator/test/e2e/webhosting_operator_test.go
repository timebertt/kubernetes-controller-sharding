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

package e2e

import (
	"context"
	"fmt"
	"maps"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

const objectCount = 100

var (
	mainObject        = &webhostingv1alpha1.Website{}
	controlledObjects = []client.Object{
		&appsv1.Deployment{},
		&corev1.ConfigMap{},
		&corev1.Service{},
		&networkingv1.Ingress{},
	}
)

var theme *webhostingv1alpha1.Theme

var _ = Describe("Webhosting Operator", Label(webhostingv1alpha1.WebhostingOperatorName), func() {
	Describe("setup", Ordered, func() {
		itDeploymentShouldBeAvailable(3)
		itControllerRingShouldHaveAvailableShards(3)
		itShouldRecognizeReadyShardLeases(3)
	})

	Describe("creating a website", Ordered, func() {
		var (
			website *webhostingv1alpha1.Website
			shard   string
		)

		itControllerRingShouldHaveAvailableShards(3)
		itShouldRecognizeReadyShardLeases(3)

		It("should assign the main object to a healthy shard", func(ctx SpecContext) {
			// Verify that the sharder successfully injects the shard label.
			// The webhook has failurePolicy=Ignore, so we might need to retry the creation until the injection succeeds.
			Eventually(ctx, func(g Gomega) *webhostingv1alpha1.Website {
				website = newWebsite("foo-" + test.RandomSuffix())
				g.Expect(testClient.Create(ctx, website)).To(Succeed())
				return website
			}).Should(And(
				HaveLabelWithValue(controllerRing.LabelShard(), BeElementOf(shards)),
				Not(HaveLabel(controllerRing.LabelDrain())),
			))
			shard = website.Labels[controllerRing.LabelShard()]

			log.Info("Created object", "website", client.ObjectKeyFromObject(website), "shard", shard)
		}, SpecTimeout(MediumTimeout))

		for _, obj := range controlledObjects {
			It(fmt.Sprintf("should assign the controlled %T to the same shard", obj), func(ctx SpecContext) {
				Eventually(ctx, ObjectList(listOf(obj), client.InNamespace(namespace.Name), client.MatchingLabels{"website": website.Name})).
					Should(HaveField("Items", ConsistOf(And(
						HaveLabelWithValue(controllerRing.LabelShard(), Equal(shard)),
						Not(HaveLabel(controllerRing.LabelDrain())),
					))))
			}, SpecTimeout(MediumTimeout))
		}

		itWebsitesShouldBeReady()
	})

	describeScaleController("adding a shard", 4)

	describeScaleController("removing a shard", 2)
})

func describeScaleController(text string, replicas int32) {
	Describe(text, Ordered, func() {
		itControllerRingShouldHaveAvailableShards(3)
		itShouldRecognizeReadyShardLeases(3)

		itCreateWebsites()
		itShouldAssignObjectsToAvailableShards()
		itWebsitesShouldBeReady()

		itScaleController(replicas)
		itDeploymentShouldBeAvailable(replicas)
		itControllerRingShouldHaveAvailableShards(replicas)
		itShouldRecognizeReadyShardLeases(int(replicas))

		itShouldAssignObjectsToAvailableShards()
		itWebsitesShouldBeReady()
	})
}

func newWebsite(name string) *webhostingv1alpha1.Website {
	labels := maps.Clone(testRunLabels)
	labels[webhostingv1alpha1.LabelKeySkipWorkload] = "true"

	return &webhostingv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Name,
			Labels:    labels,
		},
		Spec: webhostingv1alpha1.WebsiteSpec{
			Theme: theme.Name,
		},
	}
}

func itDeploymentShouldBeAvailable(expectedReplicas int32) {
	GinkgoHelper()

	It(fmt.Sprintf("the %s Deployment should be available", webhostingv1alpha1.WebhostingOperatorName), func(ctx SpecContext) {
		Eventually(ctx, Object(controllerDeployment)).Should(And(
			HaveField("Spec.Replicas", HaveValue(BeEquivalentTo(expectedReplicas))),
			HaveField("Status.Replicas", BeEquivalentTo(expectedReplicas)),
			HaveField("Status.AvailableReplicas", BeEquivalentTo(expectedReplicas)),
		))
	}, SpecTimeout(MediumTimeout))
}

func itControllerRingShouldHaveAvailableShards(expectedAvailableShards int32) {
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
	}, SpecTimeout(MediumTimeout))
}

var shards []string

func itShouldRecognizeReadyShardLeases(expectedCount int) {
	GinkgoHelper()

	It(fmt.Sprintf("should recognize %d ready shard leases", expectedCount), func(ctx SpecContext) {
		leaseList := &coordinationv1.LeaseList{}
		Eventually(ctx, ObjectList(leaseList, client.InNamespace(controllerDeployment.Namespace), client.MatchingLabels{
			shardingv1alpha1.LabelControllerRing: controllerRing.Name,
			shardingv1alpha1.LabelState:          leases.Ready.String(),
		})).Should(HaveField("Items", HaveLen(expectedCount)))

		shards = make([]string, len(leaseList.Items))
		for i, lease := range leaseList.Items {
			shards[i] = lease.Name
		}
	}, SpecTimeout(ShortTimeout))
}

func itCreateWebsites() {
	GinkgoHelper()

	It(fmt.Sprintf("create %d websites", objectCount), func(ctx SpecContext) {
		for i := 0; i < objectCount; i++ {
			website := newWebsite("foo-" + strconv.Itoa(i))
			Expect(testClient.Create(ctx, website)).To(Succeed(), "should create website %s", website.Name)
		}
	}, SpecTimeout(MediumTimeout))
}

func itWebsitesShouldBeReady() {
	GinkgoHelper()

	It("all Websites should be ready", func(ctx SpecContext) {
		Eventually(ctx, ObjectList(&webhostingv1alpha1.WebsiteList{}, client.InNamespace(namespace.Name))).Should(
			HaveField("Items", HaveEach(
				HaveField("Status.Phase", webhostingv1alpha1.PhaseReady),
			)),
		)
	}, SpecTimeout(MediumTimeout))
}

func itScaleController(replicas int32) {
	GinkgoHelper()

	It(fmt.Sprintf("scale the controller to %d replicas", replicas), func(ctx SpecContext) {
		scaleController(ctx, replicas)
	}, NodeTimeout(ShortTimeout))
}

func scaleController(ctx context.Context, replicas int32) {
	GinkgoHelper()

	patch := client.MergeFrom(&autoscalingv1.Scale{})
	scale := &autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: replicas}}
	Expect(testClient.SubResource("scale").Patch(ctx, controllerDeployment, patch, client.WithSubResourceBody(scale), &client.SubResourcePatchOptions{})).To(Succeed())

	log.Info("Scaled controller", "replicas", replicas)
}

func itShouldAssignObjectsToAvailableShards() {
	GinkgoHelper()

	for _, obj := range append([]client.Object{mainObject}, controlledObjects...) {
		It(fmt.Sprintf("should assign the %Ts to the available shards", obj), func(ctx SpecContext) {
			eventuallyShouldAssignObjectsToAvailableShards(ctx, obj)
		}, NodeTimeout(MediumTimeout))
	}
}

func eventuallyShouldAssignObjectsToAvailableShards(ctx context.Context, obj client.Object) {
	GinkgoHelper()

	Eventually(ctx, func(g Gomega) {
		list := listOf(obj)
		g.Expect(ObjectList(list, client.InNamespace(namespace.Name), client.MatchingLabels(testRunLabels))()).To(HaveField("Items", HaveLen(objectCount)))

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

func listOf(obj client.Object) client.ObjectList {
	gvk, err := apiutil.GVKForObject(obj, testClient.Scheme())
	Expect(err).NotTo(HaveOccurred())

	gvk.Kind += "List"
	list, err := testClient.Scheme().New(gvk)
	Expect(err).NotTo(HaveOccurred())
	return list.(client.ObjectList)
}
