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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebsiteSpec defines the desired state of a Website.
type WebsiteSpec struct {
	// Theme references a Theme object to be used for this Website.
	Theme string `json:"theme"`
}

// WebsiteStatus defines the observed state of a Website.
type WebsiteStatus struct {
	// The generation observed by the Website controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Phase is the current phase of this Website.
	// +optional
	Phase WebsitePhase `json:"phase,omitempty"`
}

// WebsitePhase describes the phase of a Website.
type WebsitePhase string

const (
	// PhasePending means that the Website is not ready yet.
	PhasePending WebsitePhase = "Pending"
	// PhaseReady means that the Website is ready and available.
	PhaseReady WebsitePhase = "Ready"
	// PhaseError means that there is a problem running the Website.
	PhaseError WebsitePhase = "Error"
	// PhaseTerminating means that the Website is shutting down.
	PhaseTerminating WebsitePhase = "Terminating"
)

// AllWebsitePhases is a list of all Website phases.
var AllWebsitePhases = []WebsitePhase{PhasePending, PhaseReady, PhaseError, PhaseTerminating}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Theme",type=string,JSONPath=`.spec.theme`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Website enables declarative management of hosted websites.
type Website struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec contains the specification of the desired behavior of the Website.
	// +optional
	Spec WebsiteSpec `json:"spec,omitempty"`
	// Status contains the most recently observed status of the Website.
	// +optional
	Status WebsiteStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WebsiteList contains a list of Websites.
type WebsiteList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of Websites.
	Items []Website `json:"items"`
}
