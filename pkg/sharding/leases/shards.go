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

package leases

import (
	"k8s.io/utils/clock"

	coordinationv1 "k8s.io/api/coordination/v1"
)

// Shard represents a single shard in a ring of controller instances.
type Shard struct {
	// ID is the ID of this Shard, i.e., the name of the Lease object.
	ID string
	// State is the current state of this shard based on the times and durations of the Lease object.
	State ShardState
	// Times holds the parsed times and durations of the Lease object.
	Times Times
}

// Shards is a list of Shards.
// This could also be a map, but a slice is deterministic in order. The methods returning a list are called way more
// frequently than a lookup using ByID.
type Shards []Shard

// ByID returns the Shard with the given ID from the list of Shards.
func (s Shards) ByID(id string) Shard {
	for _, shard := range s {
		if shard.ID == id {
			return shard
		}
	}

	return Shard{}
}

// AvailableShards returns the subset of available Shards as determined by IsAvailable.
func (s Shards) AvailableShards() Shards {
	shards := make(Shards, 0, len(s))
	for _, shard := range s {
		if shard.State.IsAvailable() {
			shards = append(shards, shard)
		}
	}

	return shards
}

// IDs returns the list of Shard IDs.
func (s Shards) IDs() []string {
	ids := make([]string, len(s))
	for i, shard := range s {
		ids[i] = shard.ID
	}

	return ids
}

// ToShards takes a list of Lease objects and transforms them to a list of Shards.
func ToShards(leases []coordinationv1.Lease, cl clock.PassiveClock) Shards {
	shards := make(Shards, 0, len(leases))
	for _, lease := range leases {
		l := lease
		shards = append(shards, ToShard(&l, cl))
	}
	return shards
}

// ToShard takes a Lease object and transforms it to a Shard.
func ToShard(lease *coordinationv1.Lease, cl clock.PassiveClock) Shard {
	times := ToTimes(lease, cl)
	return Shard{
		ID:    lease.GetName(),
		Times: times,
		State: toState(lease, times),
	}
}
