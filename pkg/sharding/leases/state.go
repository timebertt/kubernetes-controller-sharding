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
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
)

// ShardState represents a state of a single shard which determines whether it is available for assigning objects to it
// or whether it is unhealthy.
type ShardState int

const (
	// Unknown is the ShardState if the Lease is not present or misses required fields.
	Unknown ShardState = iota
	// Orphaned is the ShardState if the Lease has been in state Dead for at least 1 minute.
	Orphaned
	// Dead is the ShardState if the Lease is Uncertain and was successfully acquired by the sharder or if it was actively
	// released by the shard.
	Dead
	// Uncertain is the ShardState if the Lease has expired at least leaseDuration ago.
	Uncertain
	// Expired is the ShardState if the Lease has expired less than leaseDuration ago.
	Expired
	// Ready is the ShardState if the Lease is held by the shard and has not expired.
	Ready
)

// String returns a string representation of this ShardState.
func (s ShardState) String() string {
	switch s {
	case Orphaned:
		return "orphaned"
	case Dead:
		return "dead"
	case Uncertain:
		return "uncertain"
	case Expired:
		return "expired"
	case Ready:
		return "ready"
	default:
		return "unknown"
	}
}

// StateFromString returns the ShardState matching the given string representation.
func StateFromString(state string) ShardState {
	switch state {
	case "orphaned":
		return Orphaned
	case "dead":
		return Dead
	case "uncertain":
		return Uncertain
	case "expired":
		return Expired
	case "ready":
		return Ready
	default:
		return Unknown
	}
}

// IsAvailable returns true for shard states that should be considered for object assignment.
func (s ShardState) IsAvailable() bool {
	return s >= Uncertain
}

// ToState returns the ShardState of the given Lease.
func ToState(lease *coordinationv1.Lease, now time.Time) ShardState {
	return toState(lease, ToTimes(lease, now))
}

func toState(lease *coordinationv1.Lease, t Times) ShardState {
	// check if lease was released or acquired by sharder
	if holder := lease.Spec.HolderIdentity; holder == nil || *holder == "" || *holder != lease.Name {
		if t.ToOrphaned <= 0 {
			return Orphaned
		}
		return Dead
	}

	switch {
	case t.ToUncertain <= 0:
		return Uncertain
	case t.ToExpired <= 0:
		return Expired
	}

	return Ready
}
