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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.availableShards`
//+kubebuilder:printcolumn:name="Shards",type=string,JSONPath=`.status.shards`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterRing declares a virtual ring of sharded controller instances. The specified objects are distributed across
// shards of this ring on the cluster-scope (i.e., objects in all namespaces). Hence, the "Cluster" prefix.
type ClusterRing struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec contains the specification of the desired behavior of the ClusterRing.
	// +optional
	Spec ClusterRingSpec `json:"spec,omitempty"`
	// Status contains the most recently observed status of the ClusterRing.
	// +optional
	Status ClusterRingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterRingList contains a list of ClusterRings.
type ClusterRingList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of ClusterRings.
	Items []ClusterRing `json:"items"`
}

// ClusterRingSpec defines the desired state of a ClusterRing.
type ClusterRingSpec struct {
	// Resources specifies the list of resources that are distributed across shards in this ClusterRing.
	// +optional
	// +listType=map
	// +listMapKey=group
	// +listMapKey=resource
	Resources []RingResource `json:"resources,omitempty"`
}

// RingResource specifies a resource along with controlled resources that is distributed across shards in a ring.
type RingResource struct {
	// GroupResource specifies the resource that is distributed across shards in a ring.
	// This resource is the controller's main resource, i.e., the resource of which it updates the object status.
	metav1.GroupResource `json:",inline"`

	// ControlledResources are additional resources that are distributed across shards in the ClusterRing.
	// These resources are controlled by the controller's main resource, i.e., they have an owner reference with
	// controller=true back to the GroupResource of this RingResource.
	// Typically, the controller also watches objects of this resource and enqueues the owning object (of the main
	// resource) whenever the status of a controlled object changes.
	// +optional
	// +listType=map
	// +listMapKey=group
	// +listMapKey=resource
	ControlledResources []metav1.GroupResource `json:"controlledResources,omitempty"`
}

// ClusterRingStatus defines the observed state of a ClusterRing.
type ClusterRingStatus struct {
	// The generation observed by the ClusterRing controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Shards is the total number of shards of this ring.
	Shards int32 `json:"shards"`
	// AvailableShards is the total number of available shards of this ring.
	AvailableShards int32 `json:"availableShards"`
}

// LeaseSelector returns a label selector for selecting shard Lease objects belonging to this ClusterRing.
func (c *ClusterRing) LeaseSelector() labels.Selector {
	return labels.SelectorFromSet(labels.Set{LabelClusterRing: c.Name})
}

// LabelShard returns the label on sharded objects that holds the name of the responsible shard within this ClusterRing.
func (c *ClusterRing) LabelShard() string {
	return LabelShard(KindClusterRing, "", c.Name)
}

// RingResources returns the the list of resources that are distributed across shards in this ClusterRing.
func (c *ClusterRing) RingResources() []RingResource {
	return c.Spec.Resources
}
