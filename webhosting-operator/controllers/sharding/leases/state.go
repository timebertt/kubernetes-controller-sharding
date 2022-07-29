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

type ShardState string

const (
	// Unknown is the ShardState if the Lease is not present or misses required fields.
	Unknown ShardState = "unknown"
	// Dead is the ShardState if the Lease has expired more than leaseDuration ago.
	Dead ShardState = "dead"
	// Uncertain is the ShardState if the Lease has expired less than leaseDuration ago.
	Uncertain ShardState = "uncertain"
	// Ready is the ShardState if the Lease is held by the shard and has not expired.
	Ready ShardState = "ready"
)

func ShardStateFromString(state string) ShardState {
	switch state {
	case string(Dead):
		return Dead
	case string(Uncertain):
		return Uncertain
	case string(Ready):
		return Ready
	default:
		return Unknown
	}
}
