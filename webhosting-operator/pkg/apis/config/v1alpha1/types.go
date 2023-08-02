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
)

//+kubebuilder:object:root=true

// WebhostingOperatorConfig is the Schema for the controllermanagerconfigs API
type WebhostingOperatorConfig struct {
	metav1.TypeMeta `json:",inline"`

	// ClientConnection holds configuration for the kubernetes API clients.
	// +optional
	ClientConnection *componentbaseconfigv1alpha1.ClientConnectionConfiguration `json:"clientConnection,omitempty"`
	// LeaderElection is the LeaderElection config to be used when configuring
	// the manager.Manager leader election
	// +optional
	LeaderElection *componentbaseconfigv1alpha1.LeaderElectionConfiguration `json:"leaderElection,omitempty"`
	// Debugging holds configuration for Debugging related features.
	// +optional
	Debugging *componentbaseconfigv1alpha1.DebuggingConfiguration `json:"debugging,omitempty"`
	// Health contains the controller health configuration
	Health HealthEndpoint `json:"health"`
	// Metrics contains the controller metrics configuration
	Metrics MetricsEndpoint `json:"metrics"`
	// GracefulShutdownTimeout is the duration given to runnable to stop before the manager actually returns on stop.
	// To disable graceful shutdown, set it to 0s.
	// To use graceful shutdown without timeout, set to a negative duration, e.G. -1s.
	// The graceful shutdown is skipped for safety reasons in case the leader election lease is lost.
	GracefulShutdownTimeout *metav1.Duration `json:"gracefulShutDown,omitempty"`

	// Ingress specifies configuration for the Ingress objects created for Websites.
	// +optional
	Ingress *IngressConfiguration `json:"ingress,omitempty"`
}

// HealthEndpoint defines the health configs.
type HealthEndpoint struct {
	// BindAddress is the TCP address that the controller should bind to
	// for serving health probes
	// It can be set to "0" to disable serving the health probe.
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`
}

// MetricsEndpoint defines the metrics configs.
type MetricsEndpoint struct {
	// BindAddress is the TCP address that the controller should bind to
	// for serving prometheus metrics.
	// It can be set to "0" to disable the metrics serving.
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`
}

// IngressConfiguration contains configuration for the Ingress objects created for Websites.
type IngressConfiguration struct {
	// Annotations is a set of annotations to add to all created Ingress objects.
	// +optional
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
