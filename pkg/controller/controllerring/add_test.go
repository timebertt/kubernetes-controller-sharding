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

package controllerring_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/controller/controllerring"
)

var _ = Describe("Reconciler", func() {
	var r *Reconciler

	BeforeEach(func() {
		r = &Reconciler{}
	})

	Describe("#LeasePredicate", func() {
		var (
			p           predicate.Predicate
			obj, objOld *coordinationv1.Lease

			fakeClock *testing.FakePassiveClock
		)

		BeforeEach(func() {
			fakeClock = testing.NewFakePassiveClock(time.Now())
			r.Clock = fakeClock

			p = r.LeasePredicate()

			obj = &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-0",
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr.To("foo-0"),
					LeaseDurationSeconds: ptr.To[int32](10),
					AcquireTime:          ptr.To(metav1.NewMicroTime(fakeClock.Now().Add(-5 * time.Minute))),
					RenewTime:            ptr.To(metav1.NewMicroTime(fakeClock.Now().Add(-2 * time.Second))),
				},
			}
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "alpha.sharding.timebertt.dev/controllerring", "foo")
			objOld = obj.DeepCopy()
		})

		It("should ignore leases with empty label", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "alpha.sharding.timebertt.dev/controllerring", "")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
		})

		It("should react on create events", func() {
			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
		})

		It("should react on delete events", func() {
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
		})

		It("should react when shard state changed to available", func() {
			objOld.Spec.HolderIdentity = nil
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should react when shard state changed to unavailable", func() {
			obj.Spec.HolderIdentity = nil
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should ignore when shard state hasn't changed", func() {
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())

			obj.Spec.HolderIdentity = nil
			objOld.Spec.HolderIdentity = nil
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
		})
	})
})
