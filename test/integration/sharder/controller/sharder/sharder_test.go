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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/controller/sharder"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/key"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("Sharder controller", func() {
	var (
		mainObj       *corev1.Secret
		controlledObj *corev1.ConfigMap

		availableShard, deadShard *coordinationv1.Lease
	)

	BeforeEach(func(ctx SpecContext) {
		mainObj = newObject(&corev1.Secret{})
		controlledObj = newObject(&corev1.ConfigMap{})
		// We only test the controller in this test and not the webhook. Hence, the objects will not actually be assigned
		// after the controller triggers assignments. To verify that the sharder triggered assignments, we add the drain
		// label with an otherwise unused value. The sharder always removes the drain label when triggering assignments.
		// When triggering the drain operation, the sharder sets this label to another value.
		metav1.SetMetaDataLabel(&mainObj.ObjectMeta, controllerRing.LabelDrain(), "test")
		metav1.SetMetaDataLabel(&controlledObj.ObjectMeta, controllerRing.LabelDrain(), "test")

		availableShard = newLease()

		deadShard = newLease()
		deadShard.Spec.HolderIdentity = nil
		Expect(testClient.Create(ctx, deadShard)).To(Succeed())

		DeferCleanup(func(ctx SpecContext) {
			// clean up all leases from this test case
			Expect(testClient.DeleteAllOf(ctx, &coordinationv1.Lease{}, client.InNamespace(testRunID))).To(Succeed())
			// wait until the manager no longer sees leases from this test case
			Eventually(ctx, New(mgrClient).ObjectList(&coordinationv1.LeaseList{}, client.InNamespace(testRunID))).Should(HaveField("Items", BeEmpty()))
		}, NodeTimeout(time.Minute))
	}, NodeTimeout(time.Minute))

	JustBeforeEach(func(ctx SpecContext) {
		Expect(testClient.Create(ctx, mainObj)).To(Succeed())
		Expect(controllerutil.SetControllerReference(mainObj, controlledObj, testClient.Scheme())).To(Succeed())
		Expect(testClient.Create(ctx, controlledObj)).To(Succeed())
	}, NodeTimeout(time.Minute))

	When("the first shard becomes available", func() {
		JustBeforeEach(func(ctx SpecContext) {
			Expect(testClient.Create(ctx, availableShard)).To(Succeed())
		}, NodeTimeout(time.Minute))

		When("objects are not assigned", func() {
			It("should trigger assignments", func(ctx SpecContext) {
				Eventually(ctx, Object(mainObj)).Should(Not(HaveLabel(controllerRing.LabelDrain())))
				Eventually(ctx, Object(controlledObj)).Should(Not(HaveLabel(controllerRing.LabelDrain())))
			}, SpecTimeout(time.Minute))
		})

		When("objects are assigned to unavailable shard", func() {
			BeforeEach(func(ctx SpecContext) {
				metav1.SetMetaDataLabel(&mainObj.ObjectMeta, controllerRing.LabelShard(), deadShard.Name)
				metav1.SetMetaDataLabel(&controlledObj.ObjectMeta, controllerRing.LabelShard(), deadShard.Name)
			}, NodeTimeout(time.Minute))

			It("should trigger assignments", func(ctx SpecContext) {
				Eventually(ctx, Object(mainObj)).Should(And(
					Not(HaveLabel(controllerRing.LabelShard())),
					Not(HaveLabel(controllerRing.LabelDrain())),
				))
				Eventually(ctx, Object(controlledObj)).Should(And(
					Not(HaveLabel(controllerRing.LabelShard())),
					Not(HaveLabel(controllerRing.LabelDrain())),
				))
			}, SpecTimeout(time.Minute))
		})

		When("objects are not in a selected namespace", func() {
			BeforeEach(func() {
				mainObj.Namespace = metav1.NamespaceDefault
				controlledObj.Namespace = metav1.NamespaceDefault
			})

			It("should not trigger assignments", func(ctx SpecContext) {
				Consistently(ctx, Object(mainObj)).Should(HaveLabelWithValue(controllerRing.LabelDrain(), "test"))
				Consistently(ctx, Object(controlledObj)).Should(HaveLabelWithValue(controllerRing.LabelDrain(), "test"))
			}, SpecTimeout(time.Minute))
		})

		When("controlled object doesn't have a controller reference", func() {
			var uncontrolledObj *corev1.ConfigMap

			BeforeEach(func(ctx SpecContext) {
				uncontrolledObj = newObject(&corev1.ConfigMap{})
				metav1.SetMetaDataLabel(&uncontrolledObj.ObjectMeta, controllerRing.LabelDrain(), "test")
				Expect(testClient.Create(ctx, uncontrolledObj)).To(Succeed())
			}, NodeTimeout(time.Minute))

			It("should not trigger assignments", func(ctx SpecContext) {
				Eventually(ctx, Object(controlledObj)).Should(Not(HaveLabel(controllerRing.LabelDrain())))
				Consistently(ctx, Object(uncontrolledObj)).Should(HaveLabelWithValue(controllerRing.LabelDrain(), "test"))
			}, SpecTimeout(time.Minute))
		})
	})

	When("the assigned shard becomes unavailable", func() {
		var newShard *coordinationv1.Lease

		BeforeEach(func() {
			metav1.SetMetaDataLabel(&mainObj.ObjectMeta, controllerRing.LabelShard(), availableShard.Name)
			metav1.SetMetaDataLabel(&controlledObj.ObjectMeta, controllerRing.LabelShard(), availableShard.Name)

			newShard = newLease()

			// overwrite key functions to prefer first shard
			DeferCleanup(overwriteKeyFuncs(keyForShard(availableShard.Name)))
		})

		JustBeforeEach(func(ctx SpecContext) {
			Expect(testClient.Create(ctx, availableShard)).To(Succeed())
			Expect(testClient.Create(ctx, newShard)).To(Succeed())
		}, NodeTimeout(time.Minute))

		It("should immediately move the objects", func(ctx SpecContext) {
			By("assigned shard is still available")
			Consistently(ctx, Object(mainObj)).Should(HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name))
			Consistently(ctx, Object(controlledObj)).Should(HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name))

			By("assigned shard becomes unavailable")
			Eventually(ctx, Update(availableShard, func() {
				availableShard.Spec.HolderIdentity = nil
			})).Should(Succeed())

			Eventually(ctx, Object(mainObj)).Should(And(
				Not(HaveLabel(controllerRing.LabelShard())),
				Not(HaveLabel(controllerRing.LabelDrain())),
			))
			Eventually(ctx, Object(controlledObj)).Should(And(
				Not(HaveLabel(controllerRing.LabelShard())),
				Not(HaveLabel(controllerRing.LabelDrain())),
			))
		}, SpecTimeout(time.Minute))
	})

	When("another shard becomes available", func() {
		var newShard *coordinationv1.Lease

		BeforeEach(func() {
			metav1.SetMetaDataLabel(&mainObj.ObjectMeta, controllerRing.LabelShard(), availableShard.Name)
			metav1.SetMetaDataLabel(&controlledObj.ObjectMeta, controllerRing.LabelShard(), availableShard.Name)

			newShard = newLease()
			newShard.Spec.HolderIdentity = nil

			// overwrite key functions to prefer new shard
			DeferCleanup(overwriteKeyFuncs(keyForShard(newShard.Name)))
		})

		JustBeforeEach(func(ctx SpecContext) {
			Expect(testClient.Create(ctx, availableShard)).To(Succeed())
			Expect(testClient.Create(ctx, newShard)).To(Succeed())
		}, NodeTimeout(time.Minute))

		It("should drain the main object and immediately move the controlled object", func(ctx SpecContext) {
			By("only one shard is available")
			Consistently(ctx, Object(mainObj)).Should(HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name))
			Consistently(ctx, Object(controlledObj)).Should(HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name))

			By("a new shard becomes available")
			Eventually(ctx, Update(newShard, func() {
				newShard.Spec.HolderIdentity = ptr.To(newShard.Name)
			})).Should(Succeed())

			Eventually(ctx, Object(mainObj)).Should(And(
				HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name),
				HaveLabel(controllerRing.LabelDrain()),
			), "should drain the main object")
			Eventually(ctx, Object(controlledObj)).Should(And(
				Not(HaveLabel(controllerRing.LabelShard())),
				Not(HaveLabel(controllerRing.LabelDrain())),
			), "should move the controlled object")
		}, SpecTimeout(time.Minute))
	})
})

func newObject[T client.Object](obj T) T {
	obj.SetName(testRunID + "-" + test.RandomSuffix())
	obj.SetNamespace(testRunID)
	return obj
}

func newLease() *coordinationv1.Lease {
	name := testRunID + "-" + test.RandomSuffix()

	return &coordinationv1.Lease{
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
}

func overwriteKeyFuncs(fn key.Func) func() {
	origObject, origController := sharder.KeyForObject, sharder.KeyForController
	sharder.KeyForObject, sharder.KeyForController = fn, fn

	return func() {
		sharder.KeyForObject, sharder.KeyForController = origObject, origController
	}
}

// keyForShard returns a key.Func that prefers assigning all objects to the shard with the given name.
// This is done by returning the key used for the first virtual node of the shard.
func keyForShard(shardName string) key.Func {
	return func(obj client.Object) (string, error) {
		return shardName + "0", nil
	}
}
