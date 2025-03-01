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

package sharder_test

import (
	"context"
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
)

var _ = Describe("Shard Lease controller", func() {
	var (
		mainObj *corev1.ConfigMap

		availableShard, deadShard *coordinationv1.Lease
	)

	BeforeEach(func(ctx SpecContext) {
		mainObj = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-" + test.RandomSuffix(),
				Namespace: testRunID,
				Labels:    maps.Clone(testRunLabels),
			},
		}

		availableShard = newLease(controllerRing.Name)
		Expect(testClient.Create(ctx, availableShard)).To(Succeed())

		deadShard = newLease(controllerRing.Name)
		deadShard.Spec.HolderIdentity = nil
		Expect(testClient.Create(ctx, deadShard)).To(Succeed())

		DeferCleanup(func(ctx SpecContext) {
			// clean up all leases from this test case
			Expect(testClient.DeleteAllOf(ctx, &coordinationv1.Lease{}, client.InNamespace(testRunID))).To(Succeed())
			// wait until the manager no longer sees leases from this test case
			Eventually(ctx, New(mgrClient).ObjectList(&coordinationv1.LeaseList{})).Should(HaveField("Items", BeEmpty()))
		}, NodeTimeout(time.Minute))
	}, NodeTimeout(time.Minute))

	Describe("main resource", func() {
		It("should assign the object during creation", func(ctx SpecContext) {
			Eventually(ctx, createObject(mainObj)).Should(beAssigned(availableShard.Name))
		}, SpecTimeout(time.Minute))

		It("should fail due to unsupported generateName", func(ctx SpecContext) {
			mainObj.Name = ""
			mainObj.GenerateName = "test-"

			Eventually(ctx, func(ctx context.Context) error {
				return testClient.Create(ctx, mainObj, client.DryRunAll)
			}).Should(MatchError(ContainSubstring("generateName is not supported")))
		}, SpecTimeout(time.Minute))

		When("the assigned shard gets unavailable", func() {
			var (
				assignedShard, otherShard *coordinationv1.Lease
			)

			BeforeEach(func(ctx SpecContext) {
				Eventually(ctx, createObject(mainObj)).Should(beAssigned(availableShard.Name))

				Eventually(ctx, Update(availableShard, func() {
					availableShard.Spec.HolderIdentity = nil
				})).Should(Succeed())
				assignedShard = availableShard

				Eventually(ctx, Update(deadShard, func() {
					deadShard.Spec.HolderIdentity = ptr.To(deadShard.Name)
				})).Should(Succeed())
				otherShard = deadShard
			}, NodeTimeout(time.Minute))

			It("should not reassign the object during update", func(ctx SpecContext) {
				Consistently(ctx, patchObject(mainObj)).Should(beAssigned(assignedShard.Name))
			})

			It("should reassign the object during drain", func(ctx SpecContext) {
				Eventually(ctx, drainObject(mainObj)).Should(beAssigned(otherShard.Name))
			})
		})
	})

	Describe("controlled resource", func() {
		var controlledObj *corev1.Secret

		BeforeEach(func(ctx SpecContext) {
			Eventually(ctx, createObject(mainObj)).Should(beAssigned(availableShard.Name))

			controlledObj = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-" + test.RandomSuffix(),
					Namespace: testRunID,
					Labels:    maps.Clone(testRunLabels),
				},
			}

			Expect(controllerutil.SetControllerReference(mainObj, controlledObj, testClient.Scheme())).To(Succeed())
		}, NodeTimeout(time.Minute))

		It("should assign the object during creation", func(ctx SpecContext) {
			Eventually(ctx, createObject(controlledObj)).Should(beAssigned(availableShard.Name))
		}, SpecTimeout(time.Minute))

		It("should assign the object during creation with generateName", func(ctx SpecContext) {
			controlledObj.Name = ""
			controlledObj.GenerateName = "test-"

			Eventually(ctx, createObject(controlledObj)).Should(beAssigned(availableShard.Name))
		}, SpecTimeout(time.Minute))

		It("should not assign an object without controller reference", func(ctx SpecContext) {
			controlledObj.OwnerReferences = nil

			Consistently(ctx, createObject(controlledObj, client.DryRunAll)).ShouldNot(beAssigned())
		}, SpecTimeout(time.Minute))

		When("the assigned shard gets unavailable", func() {
			var (
				assignedShard, otherShard *coordinationv1.Lease
			)

			BeforeEach(func(ctx SpecContext) {
				Eventually(ctx, createObject(controlledObj)).Should(beAssigned(availableShard.Name))

				Eventually(ctx, Update(availableShard, func() {
					availableShard.Spec.HolderIdentity = nil
				})).Should(Succeed())
				assignedShard = availableShard

				Eventually(ctx, Update(deadShard, func() {
					deadShard.Spec.HolderIdentity = ptr.To(deadShard.Name)
				})).Should(Succeed())
				otherShard = deadShard
			}, NodeTimeout(time.Minute))

			It("should not reassign the object during update", func(ctx SpecContext) {
				Consistently(ctx, patchObject(controlledObj)).Should(beAssigned(assignedShard.Name))
			})

			It("should reassign the object during drain", func(ctx SpecContext) {
				Eventually(ctx, drainObject(controlledObj)).Should(beAssigned(otherShard.Name))
			})
		})
	})
})

func newLease(controllerRingName string) *coordinationv1.Lease {
	name := testRunID + "-" + test.RandomSuffix()

	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testRunID,
			Labels: map[string]string{
				shardingv1alpha1.LabelControllerRing: controllerRingName,
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(name),
			LeaseDurationSeconds: ptr.To[int32](10),
			AcquireTime:          ptr.To(metav1.NewMicroTime(clock.Now().Add(-5 * time.Minute))),
			RenewTime:            ptr.To(metav1.NewMicroTime(clock.Now().Add(-2 * time.Second))),
		},
	}
}

func beAssigned(shard ...string) gomegatypes.GomegaMatcher {
	if len(shard) == 0 {
		return HaveField("ObjectMeta.Labels",
			HaveKey("shard.alpha.sharding.timebertt.dev/"+controllerRing.Name),
		)
	}

	return HaveField("ObjectMeta.Labels",
		HaveKeyWithValue("shard.alpha.sharding.timebertt.dev/"+controllerRing.Name, shard[0]),
	)
}

func createObject(obj client.Object, opts ...client.CreateOption) func(ctx context.Context) (client.Object, error) {
	return func(ctx context.Context) (client.Object, error) {
		obj.SetResourceVersion("")

		err := testClient.Create(ctx, obj, opts...)
		if apierrors.IsAlreadyExists(err) {
			return obj, StopTrying(err.Error())
		}

		return obj, err
	}
}

func patchObject(obj client.Object) func(ctx context.Context) (client.Object, error) {
	return func(ctx context.Context) (client.Object, error) {
		return obj, testClient.Patch(ctx, obj, client.RawPatch(types.MergePatchType, []byte("{}")))
	}
}

func drainObject(obj client.Object) func(ctx context.Context) (client.Object, error) {
	patch := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	labels := obj.GetLabels()
	delete(labels, "shard.alpha.sharding.timebertt.dev/"+controllerRing.Name)
	obj.SetLabels(labels)

	return func(ctx context.Context) (client.Object, error) {
		return obj, testClient.Patch(ctx, obj, patch)
	}
}
