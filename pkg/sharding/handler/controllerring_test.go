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

package handler_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/handler"
)

var _ = Describe("ControllerRing", func() {
	var (
		ctx context.Context

		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()

		fakeClient = fakeclient.NewFakeClient()
	})

	Describe("#MapControllerRingToLeases", func() {
		var (
			mapFunc handler.MapFunc

			obj *shardingv1alpha1.ControllerRing
		)

		BeforeEach(func() {
			mapFunc = MapControllerRingToLeases(fakeClient)

			obj = &shardingv1alpha1.ControllerRing{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			}

			lease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-1",
					Namespace: "foo-system",
					Labels: map[string]string{
						"alpha.sharding.timebertt.dev/controllerring": "foo",
					},
				},
			}
			Expect(fakeClient.Create(ctx, lease.DeepCopy())).To(Succeed())

			lease.Name = "foo-2"
			Expect(fakeClient.Create(ctx, lease.DeepCopy())).To(Succeed())

			lease.Name = "foo-3"
			lease.Labels["alpha.sharding.timebertt.dev/controllerring"] = "bar"
			Expect(fakeClient.Create(ctx, lease.DeepCopy())).To(Succeed())

			lease.Name = "foo-4"
			lease.Labels = nil
			Expect(fakeClient.Create(ctx, lease.DeepCopy())).To(Succeed())
		})

		It("should ignore other object kinds", func() {
			Expect(mapFunc(ctx, &corev1.Pod{})).To(BeEmpty())
		})

		It("should return requests for all matching leases", func() {
			Expect(mapFunc(ctx, obj)).To(ConsistOf(
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo-system", Name: "foo-1"}},
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "foo-system", Name: "foo-2"}},
			))
		})
	})
})
