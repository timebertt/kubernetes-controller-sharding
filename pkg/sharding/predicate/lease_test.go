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

package predicate_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/predicate"
)

var _ = Describe("Lease", func() {
	var (
		p predicate.Predicate

		fakeClock   *testing.FakePassiveClock
		obj, objOld *coordinationv1.Lease
	)

	BeforeEach(func() {
		fakeClock = testing.NewFakePassiveClock(time.Now())

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

	Describe("#IsShardLease", func() {
		BeforeEach(func() {
			p = IsShardLease()
		})

		It("should ignore other object kinds", func() {
			pod := &corev1.Pod{}

			Expect(p.Create(event.CreateEvent{Object: pod})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: pod, ObjectNew: pod})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: pod})).To(BeFalse())
		})

		It("should ignore leases without label", func() {
			delete(obj.Labels, "alpha.sharding.timebertt.dev/controllerring")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
		})

		It("should ignore leases with empty label", func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "alpha.sharding.timebertt.dev/controllerring", "")
			objOld = obj.DeepCopy()

			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
		})

		It("should react on lease events with label", func() {
			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
		})
	})

	Describe("#ShardLeaseStateChanged", func() {
		BeforeEach(func() {
			p = ShardLeaseStateChanged(fakeClock)
		})

		It("should react on all create events", func() {
			Expect(p.Create(event.CreateEvent{})).To(BeTrue())
		})

		It("should react on all delete events", func() {
			Expect(p.Delete(event.DeleteEvent{})).To(BeTrue())
		})

		It("should ignore other object kinds on update events", func() {
			pod := &corev1.Pod{}

			Expect(p.Update(event.UpdateEvent{ObjectOld: pod, ObjectNew: pod})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: pod})).To(BeFalse())
		})

		It("should react when shard state changed to dead (lease released)", func() {
			obj.Spec.HolderIdentity = nil

			Expect(leases.ToState(objOld, fakeClock.Now())).To(Equal(leases.Ready))
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Dead))

			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should react when shard state changed to ready (renewed after expired)", func() {
			objOld.Spec.RenewTime = ptr.To(metav1.NewMicroTime(fakeClock.Now().Add(-time.Duration(*obj.Spec.LeaseDurationSeconds+1) * time.Second)))

			Expect(leases.ToState(objOld, fakeClock.Now())).To(Equal(leases.Expired))
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Ready))

			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should ignore when shard state hasn't changed", func() {
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())

			obj.Spec.HolderIdentity = nil
			objOld.Spec.HolderIdentity = nil
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
		})
	})

	Describe("#ShardLeaseAvailabilityChanged", func() {
		BeforeEach(func() {
			p = ShardLeaseAvailabilityChanged(fakeClock)
		})

		It("should react on create events if shard is available", func() {
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Ready))
			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeTrue())
		})

		It("should ignore create events if shard is not available", func() {
			obj.Spec.HolderIdentity = nil
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Dead))
			Expect(p.Create(event.CreateEvent{Object: obj})).To(BeFalse())
		})

		It("should react on delete events if shard was available", func() {
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Ready))
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeTrue())
		})

		It("should ignore delete events if shard was not available", func() {
			obj.Spec.HolderIdentity = nil
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Dead))
			Expect(p.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
		})

		It("should ignore other object kinds", func() {
			pod := &corev1.Pod{}

			Expect(p.Create(event.CreateEvent{Object: pod})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: pod, ObjectNew: pod})).To(BeFalse())
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: pod})).To(BeFalse())
			Expect(p.Delete(event.DeleteEvent{Object: pod})).To(BeFalse())
		})

		It("should react when shard state changed to dead (lease released)", func() {
			obj.Spec.HolderIdentity = nil

			Expect(leases.ToState(objOld, fakeClock.Now())).To(Equal(leases.Ready))
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Dead))

			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should ignore when shard state changed to ready (renewed after expired)", func() {
			objOld.Spec.RenewTime = ptr.To(metav1.NewMicroTime(fakeClock.Now().Add(-time.Duration(*obj.Spec.LeaseDurationSeconds+1) * time.Second)))

			Expect(leases.ToState(objOld, fakeClock.Now())).To(Equal(leases.Expired))
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Ready))

			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
		})

		It("should react when shard state changed to ready (renewed after dead)", func() {
			objOld.Spec.HolderIdentity = nil

			Expect(leases.ToState(objOld, fakeClock.Now())).To(Equal(leases.Dead))
			Expect(leases.ToState(obj, fakeClock.Now())).To(Equal(leases.Ready))

			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeTrue())
		})

		It("should ignore when shard state hasn't changed", func() {
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())

			obj.Spec.HolderIdentity = nil
			objOld.Spec.HolderIdentity = nil
			Expect(p.Update(event.UpdateEvent{ObjectOld: objOld, ObjectNew: obj})).To(BeFalse())
		})

		It("should react if final delete state is unknown even if lease was unavailable", func() {
			obj.Spec.HolderIdentity = nil

			Expect(p.Delete(event.DeleteEvent{Object: obj, DeleteStateUnknown: true})).To(BeTrue())
		})
	})
})
