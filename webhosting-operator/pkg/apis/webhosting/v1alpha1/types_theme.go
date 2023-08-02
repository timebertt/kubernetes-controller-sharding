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

// ThemeSpec defines the desired state of a Theme.
type ThemeSpec struct {
	// Color is a CSS color for a Website.
	Color string `json:"color"`
	// FontFamily is a font family for a Website.
	FontFamily string `json:"fontFamily"`
}

// ThemeStatus defines the observed state of a Theme.
type ThemeStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Color",type=string,JSONPath=`.spec.color`
//+kubebuilder:printcolumn:name="Font Family",type=string,JSONPath=`.spec.fontFamily`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Theme is the Schema for the themes API.
type Theme struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec contains the specification of the desired behavior of the Theme.
	// +optional
	Spec ThemeSpec `json:"spec,omitempty"`
	// Status contains the most recently observed status of the Theme.
	// +optional
	Status ThemeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ThemeList contains a list of Themes.
type ThemeList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is the list of Themes.
	Items []Theme `json:"items"`
}
