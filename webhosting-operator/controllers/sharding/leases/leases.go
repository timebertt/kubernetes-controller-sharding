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
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
)

func ToShardState(lease *coordinationv1.Lease, cl clock.Clock) ShardState {
	// TODO: cross-check this with leader election code
	var (
		holder          = lease.Spec.HolderIdentity
		acquireTime     = lease.Spec.AcquireTime
		renewTime       = lease.Spec.RenewTime
		durationSeconds = lease.Spec.LeaseDurationSeconds
	)

	if holder == nil || acquireTime == nil || renewTime == nil || durationSeconds == nil {
		return Unknown
	}

	if lease.Name != *holder {
		return Dead
	}

	duration := time.Duration(*durationSeconds) * time.Second

	now := cl.Now()
	if now.Before(renewTime.Add(duration)) {
		return Ready
	}
	if now.Before(renewTime.Add(2 * duration)) {
		return Uncertain
	}

	return Dead
}

func ExpirationTime(lease *coordinationv1.Lease) (time.Time, time.Duration, bool) {
	var (
		acquireTime     = lease.Spec.AcquireTime
		renewTime       = lease.Spec.RenewTime
		durationSeconds = lease.Spec.LeaseDurationSeconds
	)

	if acquireTime == nil || renewTime == nil || durationSeconds == nil {
		return time.Time{}, 0, false
	}

	duration := time.Duration(*durationSeconds) * time.Second
	return renewTime.Add(duration), duration, true
}
