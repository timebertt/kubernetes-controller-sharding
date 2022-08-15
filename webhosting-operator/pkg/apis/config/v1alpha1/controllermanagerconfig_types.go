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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	runtimeconfigv1alpha1 "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

//+kubebuilder:object:root=true

// ControllerManagerConfig is the Schema for the controllermanagerconfigs API
type ControllerManagerConfig struct {
	metav1.TypeMeta `json:",inline"`
	// ControllerManagerConfigurationSpec is the basic configuration of the operator.
	runtimeconfigv1alpha1.ControllerManagerConfigurationSpec `json:",inline"`
	// Debugging holds configuration for Debugging related features.
	// +optional
	Debugging *componentbaseconfigv1alpha1.DebuggingConfiguration `json:"debugging,omitempty"`
	// Ingress specifies configuration for the Ingress objects created for Websites.
	// +optional
	Ingress *IngressConfiguration `json:"ingress,omitempty"`
}

// IngressConfiguration contains configuration for the Ingress objects created for Websites.
type IngressConfiguration struct {
	// Annotations is a set of annotations to add to all created Ingress objects.
	Annotations map[string]string `json:"annotations,omitempty"`
	// Hosts is a list of hosts, under which Websites shall be available.
	// +optional
	Hosts []string `json:"hosts,omitempty"`
	// TLS configures TLS settings to be used on Ingress objects. Specify this to make Websites serve TLS connections for
	// the given hosts. SecretName is optional. If specified, the given Secret is expected to exist already in all project
	// namespaces. Otherwise, the Website controller will fill the Ingresses secretName field and expects the secret to be
	// created and filled by an external controller (e.g. cert-manager).
	// +optional
	TLS []networkingv1.IngressTLS `json:"tls,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ControllerManagerConfig{})
}
