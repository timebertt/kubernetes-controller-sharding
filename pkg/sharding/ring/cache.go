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
	"fmt"
	"sort"
	"sync"

	"github.com/cespare/xxhash/v2"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/consistenthash"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	shardingmetrics "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/metrics"
)

// Cache caches lease data and hash rings to reduce expensive ring calculations needed in every assignment operation
// in the sharder's webhooks or controllers.
// It rebuilds the hash ring from the given LeaseList if necessary.
type Cache interface {
	// Get returns the cached ring and shard information for a given ClusterRing.
	Get(ring client.Object, leaseList *coordinationv1.LeaseList) (*consistenthash.Ring, leases.Shards)
}

// NewCache returns a new Cache.
func NewCache() Cache {
	return &cache{
		Clock:         clock.RealClock{},
		TokensPerNode: consistenthash.DefaultTokensPerNode,
		cache:         make(map[string]element),
	}
}

type cache struct {
	Clock         clock.Clock
	TokensPerNode int

	lock  sync.RWMutex
	cache map[string]element
}

type element struct {
	checksum uint64
	ring     *consistenthash.Ring
	shards   leases.Shards
}

func (c *cache) Get(ringObj client.Object, leaseList *coordinationv1.LeaseList) (*consistenthash.Ring, leases.Shards) {
	var kind, key string

	switch ringObj.(type) {
	case *shardingv1alpha1.ClusterRing:
		kind = shardingv1alpha1.KindClusterRing
		key = ringObj.GetName()
	default:
		panic(fmt.Errorf("unexpected kind %T", ringObj))
	}

	checksum := leaseListChecksum(leaseList)

	// fast pre-check if cache is up-to-date
	c.lock.RLock()
	ring, ok := c.cache[key]
	if ok && ring.checksum == checksum {
		defer c.lock.RUnlock()
		return ring.ring, ring.shards
	}

	// upgrade lock to prepare writing
	c.lock.RUnlock()
	c.lock.Lock()
	defer c.lock.Unlock()

	// re-check if cache was updated concurrently in the meantime
	ring, ok = c.cache[key]
	if ok && ring.checksum == checksum {
		return ring.ring, ring.shards
	}

	// set cached checksum
	ring.checksum = checksum

	// determine ready shards and calculate hash ring
	ring.shards = leases.ToShards(leaseList.Items, c.Clock)
	availableShards := ring.shards.AvailableShards().IDs()
	ring.ring = consistenthash.New(consistenthash.DefaultHash, c.TokensPerNode, availableShards...)

	// write back ring to cache
	c.cache[key] = ring

	shardingmetrics.RingCalculationsTotal.WithLabelValues(kind, ringObj.GetNamespace(), ringObj.GetName()).Inc()

	return ring.ring, ring.shards
}

// leaseListChecksum calculates a fast checksum for the given LeaseList. It avoids expensive checksum calculation of all
// fields by only including uid and state of the leases. When the hash ring needs to be rebuild the checksum is
// guaranteed to change.
// This trades consistency for performance: it relies on the state label maintained by the shardlease controller to be
// up-to-date. If it is outdated, additional resyncs will need to be performed for returning assignments to a consistent
// state.
func leaseListChecksum(leaseList *coordinationv1.LeaseList) uint64 {
	ll := make([]string, len(leaseList.Items))
	for i, l := range leaseList.Items {
		ll[i] = string(l.UID) + ";" + l.Labels[shardingv1alpha1.LabelState] + ";"
	}

	// ensure stable checksum
	sort.Strings(ll)

	hash := xxhash.New()
	for _, l := range ll {
		_, _ = hash.WriteString(l)
	}

	return hash.Sum64()
}
