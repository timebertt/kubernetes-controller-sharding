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

package leases_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
)

var _ = Describe("ShardState", func() {
	Describe("#String", func() {
		It("should return the state as a string", func() {
			Expect(Unknown.String()).To(Equal("unknown"))
			Expect(Orphaned.String()).To(Equal("orphaned"))
			Expect(Dead.String()).To(Equal("dead"))
			Expect(Uncertain.String()).To(Equal("uncertain"))
			Expect(Expired.String()).To(Equal("expired"))
			Expect(Ready.String()).To(Equal("ready"))
			Expect(ShardState(-1).String()).To(Equal("unknown"))
		})
	})

	Describe("#StateFromString", func() {
		It("should return the correct state matching the string", func() {
			Expect(StateFromString("unknown")).To(Equal(Unknown))
			Expect(StateFromString("orphaned")).To(Equal(Orphaned))
			Expect(StateFromString("dead")).To(Equal(Dead))
			Expect(StateFromString("uncertain")).To(Equal(Uncertain))
			Expect(StateFromString("expired")).To(Equal(Expired))
			Expect(StateFromString("ready")).To(Equal(Ready))
			Expect(StateFromString("foo")).To(Equal(Unknown))
		})
	})

	Describe("#IsAvailable", func() {
		It("should return false for unavailable states", func() {
			Expect(Unknown.IsAvailable()).To(BeFalse())
			Expect(Orphaned.IsAvailable()).To(BeFalse())
			Expect(Dead.IsAvailable()).To(BeFalse())
		})

		It("should return true for available states", func() {
			Expect(Uncertain.IsAvailable()).To(BeTrue())
			Expect(Expired.IsAvailable()).To(BeTrue())
			Expect(Ready.IsAvailable()).To(BeTrue())
		})
	})

	Describe("#ToState", func() {
		var (
			now   time.Time
			lease *coordinationv1.Lease
		)

		BeforeEach(func() {
			now = time.Now()

			lease = &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shard",
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr.To("shard"),
					LeaseDurationSeconds: ptr.To[int32](10),
					AcquireTime:          ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Minute))),
					RenewTime:            ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Second))),
				},
			}
		})

		It("should return ready if lease has not expired", func() {
			Expect(ToState(lease, now)).To(Equal(Ready))
		})

		It("should return expired if lease has expired", func() {
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now.Add(-15 * time.Second)))
			Expect(ToState(lease, now)).To(Equal(Expired))
		})

		It("should return expired if acquireTime is missing", func() {
			lease.Spec.AcquireTime = nil
			Expect(ToState(lease, now)).To(Equal(Expired))
		})

		It("should return expired if renewTime is missing", func() {
			lease.Spec.RenewTime = nil
			Expect(ToState(lease, now)).To(Equal(Expired))
		})

		It("should return expired if leaseDuration is missing", func() {
			lease.Spec.LeaseDurationSeconds = nil
			Expect(ToState(lease, now)).To(Equal(Expired))
		})

		It("should return uncertain if lease has expired more than leaseDuration ago", func() {
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now.Add(-25 * time.Second)))
			Expect(ToState(lease, now)).To(Equal(Uncertain))
		})

		It("should return dead if lease has been released", func() {
			lease.Spec.HolderIdentity = nil
			Expect(ToState(lease, now)).To(Equal(Dead))

			lease.Spec.HolderIdentity = ptr.To("")
			Expect(ToState(lease, now)).To(Equal(Dead))
		})

		It("should return dead if lease has been acquired by sharder", func() {
			lease.Spec.HolderIdentity = ptr.To("shardlease-controller")
			lease.Spec.LeaseDurationSeconds = ptr.To[int32](20)
			lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(now))
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now))
			Expect(ToState(lease, now)).To(Equal(Dead))
		})

		It("should return orphaned if lease has been released 1 minute and leaseDuration ago", func() {
			lease.Spec.HolderIdentity = nil
			lease.Spec.LeaseDurationSeconds = ptr.To[int32](1)
			lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(now.Add(-time.Minute - time.Second)))
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now.Add(-time.Minute - time.Second)))
			Expect(ToState(lease, now)).To(Equal(Orphaned))
		})

		It("should return orphaned if lease has been acquired by sharder 1 minute and ago", func() {
			lease.Spec.HolderIdentity = ptr.To("shardlease-controller")
			lease.Spec.LeaseDurationSeconds = ptr.To[int32](20)
			lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(now.Add(-time.Minute - 20*time.Second)))
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now.Add(-time.Minute - 20*time.Second)))
			Expect(ToState(lease, now)).To(Equal(Orphaned))
		})
	})
})
