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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LeaseLock implements resourcelock.Interface based on a controller-runtime client.
// It is able to inject labels into the lock object.
type LeaseLock struct {
	LeaseKey   client.ObjectKey
	Client     client.Client
	LockConfig resourcelock.ResourceLockConfig
	Labels     map[string]string
	lease      *coordinationv1.Lease
}

var _ resourcelock.Interface = &LeaseLock{}

// Get returns the election record from a Lease spec
func (ll *LeaseLock) Get(ctx context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	ll.lease = &coordinationv1.Lease{}
	if err := ll.Client.Get(ctx, ll.LeaseKey, ll.lease); err != nil {
		return nil, nil, err
	}
	record := resourcelock.LeaseSpecToLeaderElectionRecord(&ll.lease.Spec)
	recordByte, err := json.Marshal(*record)
	if err != nil {
		return nil, nil, err
	}
	return record, recordByte, nil
}

// Create attempts to create a Lease
func (ll *LeaseLock) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	ll.lease = &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ll.LeaseKey.Name,
			Namespace: ll.LeaseKey.Namespace,
		},
		Spec: resourcelock.LeaderElectionRecordToLeaseSpec(&ler),
	}
	// don't set labels map on ll.lease directly, otherwise decoding might overwrite ll.Labels
	mergeLabels(&ll.lease.ObjectMeta, ll.Labels)
	return ll.Client.Create(ctx, ll.lease)
}

// Update will update an existing Lease spec.
func (ll *LeaseLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	if ll.lease == nil {
		return errors.New("lease not initialized, call get or create first")
	}
	mergeLabels(&ll.lease.ObjectMeta, ll.Labels)
	ll.lease.Spec = resourcelock.LeaderElectionRecordToLeaseSpec(&ler)
	return ll.Client.Update(ctx, ll.lease)
}

// RecordEvent in leader election while adding meta-data
func (ll *LeaseLock) RecordEvent(s string) {
	if ll.LockConfig.EventRecorder == nil {
		return
	}
	events := fmt.Sprintf("%v %v", ll.LockConfig.Identity, s)
	subject := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{
		Name:      ll.LeaseKey.Name,
		Namespace: ll.LeaseKey.Namespace,
	}}
	// Populate the type meta, so we don't have to get it from the schema
	subject.Kind = "Lease"
	subject.APIVersion = coordinationv1.SchemeGroupVersion.String()
	ll.LockConfig.EventRecorder.Eventf(subject, corev1.EventTypeNormal, "LeaderElection", events)
}

// Describe is used to convert details on current resource lock
// into a string
func (ll *LeaseLock) Describe() string {
	return fmt.Sprintf("%v/%v", ll.LeaseKey.Namespace, ll.LeaseKey.Name)
}

// Identity returns the Identity of the lock
func (ll *LeaseLock) Identity() string {
	return ll.LockConfig.Identity
}

func mergeLabels(obj *metav1.ObjectMeta, labels map[string]string) {
	for key, value := range labels {
		metav1.SetMetaDataLabel(obj, key, value)
	}
}
