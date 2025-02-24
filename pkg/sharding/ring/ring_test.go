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

package ring_test

import (
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/consistenthash"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/ring"
)

var _ = Describe("FromLeases", func() {
	var (
		now            time.Time
		controllerRing *shardingv1alpha1.ControllerRing
		leaseList      *coordinationv1.LeaseList
	)

	BeforeEach(func() {
		now = time.Now()
		controllerRing = &shardingv1alpha1.ControllerRing{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}

		leaseList = &coordinationv1.LeaseList{}

		leaseTemplate := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo-0",
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To("foo-0"),
				LeaseDurationSeconds: ptr.To[int32](10),
				AcquireTime:          ptr.To(metav1.NewMicroTime(now.Add(-5 * time.Minute))),
				RenewTime:            ptr.To(metav1.NewMicroTime(now.Add(-2 * time.Second))),
			},
		}

		lease := leaseTemplate.DeepCopy()
		leaseList.Items = append(leaseList.Items, *lease)

		lease = leaseTemplate.DeepCopy()
		lease.Name = "foo-1"
		lease.Spec.HolderIdentity = ptr.To("foo-1")
		leaseList.Items = append(leaseList.Items, *lease)

		lease = leaseTemplate.DeepCopy()
		lease.Name = "foo-2"
		lease.Spec.HolderIdentity = nil
		leaseList.Items = append(leaseList.Items, *lease)

		lease = leaseTemplate.DeepCopy()
		lease.Name = "foo-3"
		lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(now.Add(-time.Minute)))
		leaseList.Items = append(leaseList.Items, *lease)
	})

	It("should create a ring from the available shards", func() {
		ring, shards := FromLeases(controllerRing, leaseList, now)
		Expect(ring).NotTo(BeNil())
		Expect(shards).To(HaveLen(4))
		Expect(shards.AvailableShards().IDs()).To(ConsistOf("foo-0", "foo-1"))
		Expect(probeRingNodes(ring)).To(ConsistOf("foo-0", "foo-1"))
	})
})

func probeRingNodes(ring *consistenthash.Ring) []string {
	nodes := sets.New[string]()

	for i := 0; i < 1000; i++ {
		nodes.Insert(ring.Hash(strconv.Itoa(i)))
	}

	return nodes.UnsortedList()
}
