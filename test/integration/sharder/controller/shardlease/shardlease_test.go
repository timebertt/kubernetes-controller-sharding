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

package shardlease_test

import (
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("Shard Lease controller", func() {
	var (
		controllerRing *shardingv1alpha1.ControllerRing
		lease          *coordinationv1.Lease
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
			},
		}

		Expect(testClient.Create(ctx, controllerRing)).To(Succeed())
		log.Info("Created ControllerRing for test", "controllerRingName", controllerRing.Name)

		DeferCleanup(func(ctx SpecContext) {
			Expect(testClient.Delete(ctx, controllerRing)).To(Or(Succeed(), BeNotFoundError()))
		}, NodeTimeout(time.Minute))

		name := testRunID + "-" + test.RandomSuffix()
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testRunID,
				Labels: map[string]string{
					shardingv1alpha1.LabelControllerRing: controllerRing.Name,
				},
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To(name),
				LeaseDurationSeconds: ptr.To[int32](10),
				AcquireTime:          ptr.To(metav1.NewMicroTime(clock.Now().Add(-5 * time.Minute))),
				RenewTime:            ptr.To(metav1.NewMicroTime(clock.Now().Add(-2 * time.Second))),
			},
		}

		Expect(testClient.Create(ctx, lease)).To(Succeed())
		log.Info("Created Lease for test", "leaseName", lease.Name)
	}, NodeTimeout(time.Minute), OncePerOrdered)

	Describe("should reflect the shard state in the label", Ordered, func() {
		It("shard is ready", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(haveState("ready"))
		}, SpecTimeout(time.Minute))

		It("lease is expired", func(ctx SpecContext) {
			clock.Step(8 * time.Second)
			Eventually(ctx, Object(lease)).Should(haveState("expired"))
		}, SpecTimeout(time.Minute))

		It("expired lease is renewed", func(ctx SpecContext) {
			clock.Step(8 * time.Second)
			Eventually(ctx, Object(lease)).Should(haveState("expired"))

			Eventually(ctx, Update(lease, func() {
				lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(clock.Now()))
			})).Should(Succeed())
			Eventually(ctx, Object(lease)).Should(haveState("ready"))
		}, SpecTimeout(time.Minute))

		It("should acquire expired lease that is not renewed", func(ctx SpecContext) {
			clock.Step(10 * time.Second)
			Eventually(ctx, Object(lease)).Should(haveState("expired"))

			clock.Step(10 * time.Second)
			Eventually(ctx, Object(lease)).Should(And(
				haveState("dead"),
				HaveField("Spec.HolderIdentity", HaveValue(Equal("shardlease-controller"))),
				HaveField("Spec.LeaseDurationSeconds", HaveValue(BeEquivalentTo(20))),
				HaveField("Spec.AcquireTime.Time", BeTemporally("~", clock.Now())),
				HaveField("Spec.RenewTime.Time", BeTemporally("~", clock.Now())),
				HaveField("Spec.LeaseTransitions", HaveValue(BeEquivalentTo(1))),
			))
		}, SpecTimeout(time.Minute))

		It("dead lease is renewed", func(ctx SpecContext) {
			Eventually(ctx, Update(lease, func() {
				lease.Spec.HolderIdentity = ptr.To(lease.Name)
				lease.Spec.LeaseDurationSeconds = ptr.To[int32](10)
				lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(clock.Now()))
				lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(clock.Now()))
				*lease.Spec.LeaseTransitions++
			})).Should(Succeed())
			Eventually(ctx, Object(lease)).Should(haveState("ready"))
		}, SpecTimeout(time.Minute))

		It("should garbage collect orphaned leases", func(ctx SpecContext) {
			clock.Step(20 * time.Second)
			Eventually(ctx, Object(lease)).Should(haveState("dead"))

			clock.Step(20*time.Second + time.Minute)
			Eventually(ctx, Get(lease)).Should(BeNotFoundError())
		}, SpecTimeout(time.Minute))
	})
})

func haveState(state string) gomegatypes.GomegaMatcher {
	return HaveLabelWithValue("alpha.sharding.timebertt.dev/state", state)
}
