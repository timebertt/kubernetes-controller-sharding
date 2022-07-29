/*
Copyright 2022 Tim Ebert.

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

package cache

import (
	"strings"
	"sync"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/sharding/leases"
)

type ActualStateOfWorld struct {
	lock sync.RWMutex

	Clock clock.Clock

	shards map[string]shard
}

type shard struct {
	ID    string
	State leases.ShardState
}

func (a *ActualStateOfWorld) SetLeases(leases []coordinationv1.Lease) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.shards = make(map[string]shard)

	for _, lease := range leases {
		a.setLeaseLocked(&lease)
	}
}

func (a *ActualStateOfWorld) SetLease(lease *coordinationv1.Lease) leases.ShardState {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.setLeaseLocked(lease)
}

func (a *ActualStateOfWorld) setLeaseLocked(lease *coordinationv1.Lease) leases.ShardState {
	// TODO: fix this
	if !strings.HasPrefix(lease.Name, "webhosting-operator-") {
		return leases.Unknown
	}

	state := leases.ToShardState(lease, a.Clock)
	a.shards[lease.Name] = shard{
		ID:    lease.Name,
		State: state,
	}
	return state
}

func (a *ActualStateOfWorld) GetReadyShards() []shard {
	a.lock.RLock()
	defer a.lock.RUnlock()

	var ready []shard

	for _, s := range a.shards {
		if s.State != leases.Ready {
			continue
		}
		ready = append(ready, s)
	}

	return ready
}

func (a *ActualStateOfWorld) GetShardState(id string) leases.ShardState {
	a.lock.RLock()
	defer a.lock.RUnlock()

	return a.shards[id].State
}
