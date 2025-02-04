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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/handler"
)

var _ = Describe("Lease", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("#MapLeaseToControllerRing", func() {
		var (
			mapFunc handler.MapFunc

			obj *coordinationv1.Lease
		)

		BeforeEach(func() {
			mapFunc = MapLeaseToControllerRing

			obj = &coordinationv1.Lease{}
		})

		It("should ignore unlabelled leases", func() {
			Expect(mapFunc(ctx, obj)).To(BeEmpty())
		})

		It("should ignore leases with empty label", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "alpha.sharding.timebertt.dev/controllerring", "")
			Expect(mapFunc(ctx, obj)).To(BeEmpty())
		})

		It("should correctly map leases with present label", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "alpha.sharding.timebertt.dev/controllerring", "foo")
			Expect(mapFunc(ctx, obj)).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKey{Name: "foo"}}))
		})
	})
})
