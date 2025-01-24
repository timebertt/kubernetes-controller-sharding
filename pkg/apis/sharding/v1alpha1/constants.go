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

import (
	"crypto/sha256"
	"encoding/hex"

	"k8s.io/utils/strings"
)

// This file contains API-related constants for the sharding implementation, e.g. well-known annotations and labels.

const (
	// NamespaceSystem is the namespace where the sharding system components run.
	NamespaceSystem = "sharding-system"
	// AppControllerSharding is the value for the "app.kubernetes.io/name" label used for objects related to controller
	// sharding.
	AppControllerSharding = "controller-sharding"

	// alphaPrefix is a common prefix for all well-known annotations and labels in this API version package.
	alphaPrefix = "alpha.sharding.timebertt.dev/"

	// LabelControllerRing is the label on objects that identifies the ControllerRing that the object belongs to.
	LabelControllerRing = alphaPrefix + "controllerring"
	// LabelState is the label on Lease objects that reflects the state of a shard for observability purposes.
	// This label is maintained by the shardlease controller.
	LabelState = alphaPrefix + "state"
	// LabelShardPrefix is the qualified prefix for a label on sharded objects that holds the name of the responsible
	// shard within a ring. Use LabelShard to compute the full label key for a ring.
	LabelShardPrefix = "shard." + alphaPrefix
	// LabelDrainPrefix is the qualified prefix for a label on sharded objects that instructs the responsible shard within
	// a ring to stop reconciling the object and remove both the shard and drain label. Use LabelDrain to compute the full
	// label key for a ring.
	LabelDrainPrefix = "drain." + alphaPrefix

	// IdentityShardLeaseController is the identity that the shardlease controller uses to acquire leases of unavailable
	// shards.
	IdentityShardLeaseController = "shardlease-controller"

	delimiter = "-"
)

// LabelShard returns the label on sharded objects that holds the name of the responsible shard within a ring.
func LabelShard(ringName string) string {
	return LabelShardPrefix + RingSuffix(ringName)
}

// LabelDrain returns the label on sharded objects that instructs the responsible shard within a ring to stop reconciling
// the object and remove both the shard and drain label.
func LabelDrain(ringName string) string {
	return LabelDrainPrefix + RingSuffix(ringName)
}

// RingSuffix returns the label key for a given ring name that is appended to a qualified prefix.
func RingSuffix(ringName string) string {
	keyHash := sha256.Sum256([]byte(ringName))
	hexHash := hex.EncodeToString(keyHash[:])

	// the label part after the "/" must not exceed 63 characters, cut off at 63 characters
	return strings.ShortenString(hexHash[:8]+delimiter+ringName, 63)
}
