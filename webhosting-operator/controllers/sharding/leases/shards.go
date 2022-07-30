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

package leases

import (
	"strings"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
)

type Shard struct {
	ID    string
	State ShardState
	Times Times
}

type Shards []Shard

func (s Shards) ById(id string) Shard {
	for _, shard := range s {
		if shard.ID == id {
			return shard
		}
	}

	return Shard{}
}

func (s Shards) ByState(minState ShardState) Shards {
	var shards Shards
	for _, shard := range s {
		if shard.State >= minState {
			shards = append(shards, shard)
		}
	}

	return shards
}

func (s Shards) IDs() []string {
	ids := make([]string, len(s))
	for i, shard := range s {
		ids[i] = shard.ID
	}

	return ids
}

func ToShards(leases []coordinationv1.Lease, cl clock.Clock) Shards {
	var shards Shards
	for _, lease := range leases {
		// TODO: fix this
		if !strings.HasPrefix(lease.Name, "webhosting-operator-") {
			continue
		}

		shards = append(shards, ToShard(&lease, cl))
	}
	return shards
}

func ToShard(lease *coordinationv1.Lease, cl clock.Clock) Shard {
	times := ToTimes(lease, cl)
	return Shard{
		ID:    lease.GetName(),
		Times: times,
		State: toState(lease, times),
	}
}
