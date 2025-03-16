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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/controller/sharder"
	utilclient "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/client"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
)

var (
	ctx        context.Context
	clock      *testing.FakePassiveClock
	fakeClient client.Client
	r          *Reconciler

	controllerRing *shardingv1alpha1.ControllerRing
	config         *configv1alpha1.SharderConfig

	namespace *corev1.Namespace

	availableShard *coordinationv1.Lease
)

var _ = BeforeEach(func() {
	ctx = context.Background()
	clock = testing.NewFakePassiveClock(time.Now())

	namespace = newNamespace("test")

	controllerRing = &shardingv1alpha1.ControllerRing{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Spec: shardingv1alpha1.ControllerRingSpec{
			Resources: []shardingv1alpha1.RingResource{{
				GroupResource: metav1.GroupResource{
					Resource: "secrets",
				},
				ControlledResources: []metav1.GroupResource{{
					Resource: "configmaps",
				}},
			}},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      corev1.LabelMetadataName,
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{namespace.Name},
				}},
			},
		},
	}

	availableShard = newLease()

	fakeClient = fake.NewClientBuilder().
		WithScheme(utilclient.SharderScheme).
		WithObjects(controllerRing, namespace, availableShard).
		Build()
	SetClient(fakeClient)

	config = &configv1alpha1.SharderConfig{}
	utilclient.SharderScheme.Default(config)

	r = &Reconciler{
		Client: fakeClient,
		Reader: fakeClient,
		Clock:  clock,
		Config: config,
	}
})

var _ = Describe("#GetSelectedNamespaces", func() {
	BeforeEach(func() {
		Expect(fakeClient.Create(ctx, newNamespace("other"))).To(Succeed())
		Expect(fakeClient.Create(ctx, newNamespace(metav1.NamespaceSystem))).To(Succeed())
		Expect(fakeClient.Create(ctx, newNamespace(shardingv1alpha1.NamespaceSystem))).To(Succeed())
	})

	It("should return all matching namespaces", func() {
		namespaces, err := r.GetSelectedNamespaces(ctx, controllerRing)
		Expect(err).NotTo(HaveOccurred())
		Expect(namespaces.UnsortedList()).To(ConsistOf("test"))
	})

	When("the ControllerRing's namespace selector is unset", func() {
		BeforeEach(func() {
			controllerRing.Spec.NamespaceSelector = nil
		})

		It("should use the config's namespace selector", func() {
			namespaces, err := r.GetSelectedNamespaces(ctx, controllerRing)
			Expect(err).NotTo(HaveOccurred())
			Expect(namespaces.UnsortedList()).To(ConsistOf("test", "other"))
		})
	})

	When("the ControllerRing's namespace selector is invalid", func() {
		BeforeEach(func() {
			controllerRing.Spec.NamespaceSelector.MatchExpressions[0].Operator = "invalid"
		})

		It("should return a terminal error", func() {
			Expect(r.GetSelectedNamespaces(ctx, controllerRing)).Error().To(MatchError(reconcile.TerminalError(nil)))
		})
	})
})

var _ = Describe("#NewOperation", func() {
	var deadShard *coordinationv1.Lease

	BeforeEach(func() {
		deadShard = newLease()
		deadShard.Spec.HolderIdentity = nil
		Expect(fakeClient.Create(ctx, deadShard)).To(Succeed())
	})

	It("should collect the shard leases", func() {
		o, err := r.NewOperation(ctx, controllerRing)
		Expect(err).NotTo(HaveOccurred())

		Expect(o.Shards.IDs()).To(ConsistOf(availableShard.Name, deadShard.Name))
		Expect(o.Shards.ByID(availableShard.Name).State.IsAvailable()).To(BeTrue())
		Expect(o.Shards.ByID(deadShard.Name).State.IsAvailable()).To(BeFalse())
	})

	It("should construct a hash ring", func() {
		o, err := r.NewOperation(ctx, controllerRing)
		Expect(err).NotTo(HaveOccurred())

		Expect(o.HashRing.Hash("foo")).To(Equal(availableShard.Name))
	})

	It("should collect all matching namespaces", func() {
		o, err := r.NewOperation(ctx, controllerRing)
		Expect(err).NotTo(HaveOccurred())

		Expect(o.Namespaces.UnsortedList()).To(ConsistOf("test"))
	})
})

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{corev1.LabelMetadataName: name},
	}}
}

func newLease() *coordinationv1.Lease {
	name := "shard-" + test.RandomSuffix()

	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace.Namespace,
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
