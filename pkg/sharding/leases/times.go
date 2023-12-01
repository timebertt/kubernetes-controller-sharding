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

const (
	defaultLeaseDuration = 15 * time.Second
	leaseTTL             = time.Minute
)

// Times holds the parsed times and durations of the Lease object.
type Times struct {
	// Expiration is the time when the Lease expires (RenewTime + LeaseDurationSeconds)
	Expiration time.Time
	// LeaseDuration is LeaseDurationSeconds represented as a Duration.
	LeaseDuration time.Duration

	// ToExpired is the duration until the Lease expires (Expiration - now).
	ToExpired time.Duration
	// ToUncertain is the duration until the Shard Lease becomes uncertain and should get acquired by the sharder
	// (ToExpired + LeaseDuration).
	ToUncertain time.Duration
	// ToOrphaned is the duration until the Lease becomes orphaned and should get cleaned up (ToExpired + leaseTTL).
	ToOrphaned time.Duration
}

// ToTimes parses the times and durations in the given Lease object and returns them in the Times representation.
func ToTimes(lease *coordinationv1.Lease, now time.Time) Times {
	var (
		t               = Times{}
		acquireTime     = lease.Spec.AcquireTime
		renewTime       = lease.Spec.RenewTime
		durationSeconds = lease.Spec.LeaseDurationSeconds
	)

	if acquireTime == nil || renewTime == nil || durationSeconds == nil {
		t.Expiration = now
		t.LeaseDuration = defaultLeaseDuration
	} else {
		t.LeaseDuration = time.Duration(*durationSeconds) * time.Second
		t.Expiration = renewTime.Add(t.LeaseDuration)
	}

	t.ToExpired = t.Expiration.Sub(now)
	t.ToUncertain = t.ToExpired + t.LeaseDuration
	// ToOrphaned only applies, if lease is released or acquired by sharder
	t.ToOrphaned = t.ToExpired + leaseTTL

	return t
}
