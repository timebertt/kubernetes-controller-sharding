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

const (
	defaultLeaseDuration = 15 * time.Second
	leaseTTL             = time.Minute
)

type Times struct {
	Expiration    time.Time
	LeaseDuration time.Duration

	ToExpired   time.Duration
	ToUncertain time.Duration
	ToOrphaned  time.Duration
}

func ToTimes(lease *coordinationv1.Lease, cl clock.Clock) Times {
	var (
		t               = Times{}
		now             = cl.Now()
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
	// ToOrphaned only applies, if lease is released or acquired by sharded
	t.ToOrphaned = t.ToExpired + leaseTTL

	return t
}
