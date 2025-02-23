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

var _ = Describe("Times", func() {
	Describe("#ToTimes", func() {
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

		It("should return the correct times for a ready shard", func() {
			matchTimes(ToTimes(lease, now), Times{
				Expiration:    now.Add(8 * time.Second),
				LeaseDuration: 10 * time.Second,
				ToExpired:     8 * time.Second,
				ToUncertain:   18 * time.Second,
				ToOrphaned:    8*time.Second + time.Minute,
			})
		})

		It("should return the correct times for a lease acquired by the sharder", func() {
			lease.Spec.HolderIdentity = ptr.To("shardlease-controller")
			lease.Spec.LeaseDurationSeconds = ptr.To[int32](20)
			lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Second)))
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Second)))

			matchTimes(ToTimes(lease, now), Times{
				Expiration:    now.Add(18 * time.Second),
				LeaseDuration: 20 * time.Second,
				ToExpired:     18 * time.Second,
				ToUncertain:   38 * time.Second,
				ToOrphaned:    18*time.Second + time.Minute,
			})
		})

		It("should return the correct times for a released lease", func() {
			lease.Spec.HolderIdentity = nil
			lease.Spec.LeaseDurationSeconds = ptr.To[int32](1)
			lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(now))
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now))

			matchTimes(ToTimes(lease, now), Times{
				Expiration:    now.Add(time.Second),
				LeaseDuration: time.Second,
				ToExpired:     time.Second,
				ToUncertain:   2 * time.Second,
				ToOrphaned:    time.Second + time.Minute,
			})
		})

		It("should set expiration to now if some field is missing", func() {
			expectedTimes := Times{
				Expiration:    now,
				LeaseDuration: 15 * time.Second,
				ToExpired:     0,
				ToUncertain:   15 * time.Second,
				ToOrphaned:    time.Minute,
			}

			brokenLease := lease.DeepCopy()
			brokenLease.Spec.AcquireTime = nil
			matchTimes(ToTimes(brokenLease, now), expectedTimes)

			brokenLease = lease.DeepCopy()
			brokenLease.Spec.RenewTime = nil
			matchTimes(ToTimes(brokenLease, now), expectedTimes)

			brokenLease = lease.DeepCopy()
			brokenLease.Spec.LeaseDurationSeconds = nil
			matchTimes(ToTimes(brokenLease, now), expectedTimes)
		})
	})
})

func matchTimes(actual, expected Times) {
	GinkgoHelper()

	Expect(actual.Expiration.String()).To(Equal(expected.Expiration.String()), "should match expected Expiration time")
	Expect(actual.LeaseDuration.String()).To(Equal(expected.LeaseDuration.String()), "should match expected LeaseDuration duration")
	Expect(actual.ToExpired.String()).To(Equal(expected.ToExpired.String()), "should match expected ToExpired duration")
	Expect(actual.ToUncertain.String()).To(Equal(expected.ToUncertain.String()), "should match expected ToUncertain duration")
	Expect(actual.ToOrphaned.String()).To(Equal(expected.ToOrphaned.String()), "should match expected ToOrphaned duration")
}
