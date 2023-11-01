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

package v1alpha1

// This file contains API-related constants for the sharding implementation, e.g. well-known annotations and labels.

const (
	// alphaPrefix is a common prefix for all well-known annotations and labels.
	// We use the kubernetes.io domain here for interoperability with other approaches evaluated in the thesis.
	// If we want to make the approach implemented in this repository production-ready, this should eventually be changed
	// to "alpha.sharding.timebertt.dev".
	alphaPrefix = "sharding.alpha.kubernetes.io/"

	// LabelClusterRing is the label on Lease objects that identifies the ClusterRing that the shard belongs to.
	LabelClusterRing = alphaPrefix + "clusterring"
	// LabelState is the label on Lease objects that reflects the state of a shard for observability purposes.
	// This label is maintained by the shardlease controller.
	LabelState = alphaPrefix + "state"
	// LabelShard is the label on sharded objects that holds the name of the responsible shard.
	LabelShard = alphaPrefix + "shard"
	// LabelDrain is the label on sharded objects that instructs the shard to stop reconciling the object and remove both
	// the shard and drain label.
	LabelDrain = alphaPrefix + "drain"

	// IdentityShardLeaseController is the identity that the shardlease controller uses to acquire leases of unavailable
	// shards.
	IdentityShardLeaseController = "shardlease-controller"
)
