/*
Copyright 2023 Tim Ebert.

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

package ring

import (
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"

	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/consistenthash"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	shardingmetrics "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/metrics"
)

// FromLeases creates a ring from the given membership information (shard leases). It transforms shard leases into a
// usable form, i.e., a hash ring and leases.Shards.
// This is a central function in the sharding implementation bringing together the leases package with the
// consistenthash package.
// In short, it determines the subset of available shards and constructs a new consistenthash.Ring with it.
func FromLeases(ringObj sharding.Ring, leaseList *coordinationv1.LeaseList, now time.Time) (*consistenthash.Ring, leases.Shards) {
	// determine ready shards and calculate hash ring
	shards := leases.ToShards(leaseList.Items, now)
	availableShards := shards.AvailableShards().IDs()
	ring := consistenthash.New(nil, 0, availableShards...)

	shardingmetrics.RingCalculationsTotal.WithLabelValues(ringObj.GetName()).Inc()

	return ring, shards
}
