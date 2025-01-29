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
//+kubebuilder:printcolumn:name="Ready",type="string",JSONPath=`.status.conditions[?(@.type == "Ready")].status`
//+kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.availableShards`
//+kubebuilder:printcolumn:name="Shards",type=string,JSONPath=`.status.shards`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=`.metadata.creationTimestamp`
//+kubebuilder:validation:XValidation:rule="size(self.metadata.name) <= 63",message="ControllerRing name must not be longer than 63 characters"

// ControllerRing declares a virtual ring of sharded controller instances. Objects of the specified resources are
// distributed across shards of this ring. Objects in all namespaces are considered unless a namespaceSelector is
// specified.
type ControllerRing struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec contains the specification of the desired behavior of the ControllerRing.
	// +optional
	Spec ControllerRingSpec `json:"spec,omitempty"`
	// Status contains the most recently observed status of the ControllerRing.
	// +optional
	Status ControllerRingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ControllerRingList contains a list of ControllerRings.
type ControllerRingList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of ControllerRings.
	Items []ControllerRing `json:"items"`
}

// ControllerRingSpec defines the desired state of a ControllerRing.
type ControllerRingSpec struct {
	// Resources specifies the list of resources that are distributed across shards in this ControllerRing.
	// +optional
	// +listType=map
	// +listMapKey=group
	// +listMapKey=resource
	Resources []RingResource `json:"resources,omitempty"`
	// NamespaceSelector overwrites the webhook configs' namespaceSelector.
	// If set, this selector should exclude the kube-system and sharding-system namespaces.
	// If omitted, the default namespaceSelector from the SharderConfig is used.
	// Note: changing/unsetting this selector will not remove labels from objects in namespaces that were previously
	// included.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// RingResource specifies a resource along with controlled resources that is distributed across shards in a ring.
type RingResource struct {
	// GroupResource specifies the resource that is distributed across shards in a ring.
	// This resource is the controller's main resource, i.e., the resource of which it updates the object status.
	metav1.GroupResource `json:",inline"`

	// ControlledResources are additional resources that are distributed across shards in the ControllerRing.
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

const (
	// ControllerRingReady is the condition type for the "Ready" condition on ControllerRings.
	ControllerRingReady = "Ready"
)

// ControllerRingStatus defines the observed state of a ControllerRing.
type ControllerRingStatus struct {
	// The generation observed by the ControllerRing controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Shards is the total number of shards of this ring.
	Shards int32 `json:"shards"`
	// AvailableShards is the total number of available shards of this ring.
	AvailableShards int32 `json:"availableShards"`
	// Conditions represents the observations of a foo's current state.
	// Known .status.conditions.type are: "Available", "Progressing", and "Degraded"
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// LeaseSelector returns a label selector for selecting shard Lease objects belonging to this ControllerRing.
func (c *ControllerRing) LeaseSelector() labels.Selector {
	return labels.SelectorFromSet(labels.Set{LabelControllerRing: c.Name})
}

// LabelShard returns the label on sharded objects that holds the name of the responsible shard within this ControllerRing.
func (c *ControllerRing) LabelShard() string {
	return LabelShard(c.Name)
}

// LabelDrain returns the label on sharded objects that instructs the responsible shard within this ControllerRing to stop
// reconciling the object and remove both the shard and drain label.
func (c *ControllerRing) LabelDrain() string {
	return LabelDrain(c.Name)
}

// RingResources returns the the list of resources that are distributed across shards in this ControllerRing.
func (c *ControllerRing) RingResources() []RingResource {
	return c.Spec.Resources
}
