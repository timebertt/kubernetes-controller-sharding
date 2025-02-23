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
	"github.com/onsi/gomega/gstruct"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
)

var _ = Describe("Shards", func() {
	Describe("#ByID", func() {
		It("should return the matching shard", func() {
			shards := Shards{
				{ID: "shard-1"},
				{ID: "shard-2"},
				{ID: "shard-3"},
			}
			Expect(shards.ByID("shard-2").ID).To(Equal("shard-2"))
		})

		It("should return a zero shard if the ID has not been found", func() {
			Expect(Shards{{ID: "shard-1"}}.ByID("shard-2").ID).To(BeZero())
			Expect(Shards{}.ByID("shard-2").ID).To(BeZero())
			Expect(Shards(nil).ByID("shard-2").ID).To(BeZero())
		})
	})

	Describe("#AvailableShards", func() {
		It("should return the available shards", func() {
			shards := Shards{
				{ID: "shard-1", State: Ready},
				{ID: "shard-2", State: Expired},
				{ID: "shard-3", State: Uncertain},
				{ID: "shard-4", State: Dead},
				{ID: "shard-5", State: Orphaned},
			}
			Expect(shards.AvailableShards().IDs()).To(ConsistOf("shard-1", "shard-2", "shard-3"))
		})
	})

	Describe("#IDs", func() {
		It("should return the shard IDs", func() {
			shards := Shards{
				{ID: "shard-1"},
				{ID: "shard-2"},
			}
			Expect(shards.IDs()).To(ConsistOf("shard-1", "shard-2"))
		})
	})

	Describe("#ToShards", func() {
		var (
			now   time.Time
			lease *coordinationv1.Lease
		)

		BeforeEach(func() {
			now = time.Now()

			lease = &coordinationv1.Lease{
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr.To("shard"),
					LeaseDurationSeconds: ptr.To[int32](10),
					AcquireTime:          ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Minute))),
					RenewTime:            ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Second))),
				},
			}
		})

		It("should correctly transform the lease objects", func() {
			leases := make([]coordinationv1.Lease, 2)

			leases[0] = *lease.DeepCopy()
			leases[0].Name = "shard-1"
			leases[0].Spec.HolderIdentity = ptr.To("shard-1")

			leases[1] = *lease.DeepCopy()
			leases[1].Name = "shard-2"
			leases[1].Spec.HolderIdentity = nil
			leases[1].Spec.LeaseDurationSeconds = ptr.To[int32](1)
			leases[1].Spec.AcquireTime = ptr.To(metav1.NewMicroTime(now))
			leases[1].Spec.RenewTime = ptr.To(metav1.NewMicroTime(now))

			Expect(ToShards(leases, now)).To(ConsistOf(
				gstruct.MatchAllFields(gstruct.Fields{
					"ID":    Equal("shard-1"),
					"State": Equal(Ready),
					"Times": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Expiration": Equal(now.Add(8 * time.Second)),
					}),
				}),
				gstruct.MatchAllFields(gstruct.Fields{
					"ID":    Equal("shard-2"),
					"State": Equal(Dead),
					"Times": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"ToOrphaned": Equal(time.Second + time.Minute),
					}),
				}),
			))
		})
	})
})
