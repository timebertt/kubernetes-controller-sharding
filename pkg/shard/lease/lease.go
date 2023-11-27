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

package lease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// Options provides the required configuration to create a new shard lease.
type Options struct {
	// ClusterRingName specifies the name of the ClusterRing that the shard belongs to.
	ClusterRingName string
	// LeaseNamespace determines the namespace in which the shard lease will be created.
	// Defaults to the pod's namespace if running in-cluster.
	LeaseNamespace string
	// ShardName determines the name of the shard. This will be used as the lease name as well as the lease's
	// holderIdentity.
	// Defaults to os.Hostname().
	ShardName string
}

// NewResourceLock returns a new resource lock that implements the shard lease.
// Pass this to the leader elector, e.g., leaderelection.LeaderElectionConfig.Lock (if using plain client-go) or
// manager.Options.LeaderElectionResourceLockInterface (if using controller-runtime).
func NewResourceLock(config *rest.Config, eventRecorder resourcelock.EventRecorder, options Options) (resourcelock.Interface, error) {
	// Construct client for leader election
	rest.AddUserAgent(config, "shard-lease")

	// We use plain client-go here instead of controller-runtime to avoid pulling in the controller-runtime dependency
	// for projects that want to use this helper but don't use controller-runtime.
	coordinationClient, err := coordinationv1client.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// default shard name to hostname if not set
	if options.ShardName == "" {
		options.ShardName, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("error getting hostname: %w", err)
		}
	}

	// default namespace to pod's namespace if running in-cluster
	if options.LeaseNamespace == "" {
		options.LeaseNamespace, err = getInClusterNamespace()
		if err != nil {
			return nil, fmt.Errorf("unable to determine shard lease namespace: %w", err)
		}
	}

	ll := &LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Namespace: options.LeaseNamespace,
			Name:      options.ShardName,
		},
		Client: coordinationClient,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: options.ShardName,
			// eventRecorder is optional
			EventRecorder: eventRecorder,
		},
		Labels: map[string]string{
			shardingv1alpha1.LabelClusterRing: options.ClusterRingName,
		},
	}

	return ll, nil
}

// LeaseLock implements resourcelock.Interface but is able to add labels to the Lease.
// The implementation is based resourclock.LeaseLock and shares some helper functions with it.
// nolint:revive // don't rename LeaseLock to stay close to the original implementation in client-go.
type LeaseLock struct {
	// LeaseMeta should contain a Name and a Namespace of a
	// LeaseMeta object that the LeaderElector will attempt to lead.
	LeaseMeta  metav1.ObjectMeta
	Client     coordinationv1client.LeasesGetter
	LockConfig resourcelock.ResourceLockConfig
	Labels     map[string]string
	lease      *coordinationv1.Lease
}

// Get returns the election record from a Lease spec
func (ll *LeaseLock) Get(ctx context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	lease, err := ll.Client.Leases(ll.LeaseMeta.Namespace).Get(ctx, ll.LeaseMeta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	ll.lease = lease
	record := resourcelock.LeaseSpecToLeaderElectionRecord(&ll.lease.Spec)
	recordByte, err := json.Marshal(*record)
	if err != nil {
		return nil, nil, err
	}
	return record, recordByte, nil
}

// Create attempts to create a Lease
func (ll *LeaseLock) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	var err error
	ll.lease, err = ll.Client.Leases(ll.LeaseMeta.Namespace).Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ll.LeaseMeta.Name,
			Namespace: ll.LeaseMeta.Namespace,
			Labels:    ll.Labels,
		},
		Spec: resourcelock.LeaderElectionRecordToLeaseSpec(&ler),
	}, metav1.CreateOptions{})
	return err
}

// Update will update an existing Lease spec.
func (ll *LeaseLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	if ll.lease == nil {
		return errors.New("lease not initialized, call get or create first")
	}
	// don't set labels map on ll.lease directly, otherwise we might overwrite labels managed by the sharder
	mergeLabels(&ll.lease.ObjectMeta, ll.Labels)
	ll.lease.Spec = resourcelock.LeaderElectionRecordToLeaseSpec(&ler)

	lease, err := ll.Client.Leases(ll.LeaseMeta.Namespace).Update(ctx, ll.lease, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	ll.lease = lease
	return nil
}

// RecordEvent in leader election while adding meta-data
func (ll *LeaseLock) RecordEvent(s string) {
	if ll.LockConfig.EventRecorder == nil {
		return
	}
	events := fmt.Sprintf("%v %v", ll.LockConfig.Identity, s)
	subject := &coordinationv1.Lease{ObjectMeta: ll.lease.ObjectMeta}
	// Populate the type meta, so we don't have to get it from the schema
	subject.Kind = "Lease"
	subject.APIVersion = coordinationv1.SchemeGroupVersion.String()
	ll.LockConfig.EventRecorder.Eventf(subject, corev1.EventTypeNormal, "LeaderElection", events)
}

// Describe is used to convert details on current resource lock
// into a string
func (ll *LeaseLock) Describe() string {
	return fmt.Sprintf("%v/%v", ll.LeaseMeta.Namespace, ll.LeaseMeta.Name)
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

const inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

func getInClusterNamespace() (string, error) {
	// Check whether the namespace file exists.
	// If not, we are not running in cluster so can't guess the namespace.
	if _, err := os.Stat(inClusterNamespacePath); os.IsNotExist(err) {
		return "", fmt.Errorf("not running in-cluster, please specify LeaseNamespace")
	} else if err != nil {
		return "", fmt.Errorf("error checking namespace file: %w", err)
	}

	// Load the namespace file and return its content
	namespace, err := os.ReadFile(inClusterNamespacePath)
	if err != nil {
		return "", fmt.Errorf("error reading namespace file: %w", err)
	}
	return string(namespace), nil
}
