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
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
)

type ShardState int

const (
	// Unknown is the ShardState if the Lease is not present or misses required fields.
	Unknown ShardState = iota
	// Orphaned is the ShardState if the Lease has been in state Dead for 1 minute.
	Orphaned
	// Dead is the ShardState if the Lease is Uncertain and was successfully acquired by the sharder.
	Dead
	// Uncertain is the ShardState if the Lease has expired more than leaseDuration ago.
	Uncertain
	// Expired is the ShardState if the Lease has expired less than leaseDuration ago.
	Expired
	// Ready is the ShardState if the Lease is held by the shard and has not expired.
	Ready
)

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

func ToState(lease *coordinationv1.Lease, cl clock.Clock) ShardState {
	return toState(lease, ToTimes(lease, cl))
}

// TODO: cross-check this with leader election code
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
