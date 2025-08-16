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
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
)

//+kubebuilder:object:root=true

// SharderConfig is the configuration object for the sharder component.
type SharderConfig struct {
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
	// Controller configures the sharder's controllers.
	Controller Controller `json:"controller"`
	// Webhook configures webhooks and the webhook server.
	Webhook Webhook `json:"webhook"`
	// GracefulShutdownTimeout is the duration given to runnable to stop before the manager actually returns on stop.
	// To disable graceful shutdown, set it to 0s.
	// To use graceful shutdown without timeout, set to a negative duration, e.G. -1s.
	// The graceful shutdown is skipped for safety reasons in case the leader election lease is lost.
	// Defaults to 15s
	GracefulShutdownTimeout *metav1.Duration `json:"gracefulShutDown,omitempty"`
}

// HealthEndpoint defines the health configs.
type HealthEndpoint struct {
	// BindAddress is the TCP address that the controller should bind to
	// for serving health probes
	// It can be set to "0" to disable serving the health probe.
	// Defaults to :8081
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`
}

// MetricsEndpoint defines the metrics configs.
type MetricsEndpoint struct {
	// BindAddress is the TCP address that the controller should bind to
	// for serving prometheus metrics.
	// It can be set to "0" to disable the metrics serving.
	// Defaults to :8080
	// +optional
	BindAddress string `json:"bindAddress,omitempty"`
}

// Controller configures the sharder's controllers.
type Controller struct {
	// Sharder configures the sharder controller.
	// +optional
	Sharder *SharderController `json:"sharder,omitempty"`
}

// SharderController configures the sharder controller.
type SharderController struct {
	// SyncPeriod configures how often a periodic resync of all object assignments in a ring is performed.
	// Defaults to 5m
	// +optional
	SyncPeriod *metav1.Duration `json:"syncPeriod,omitempty"`
	// ConcurrentMoves configures how many objects of the same ControllerRing are moved (or drained) concurrently at
	// maximum.
	// Defaults to 100
	// +optional
	ConcurrentMoves *int32 `json:"concurrentMoves,omitempty"`
}

// Webhook configures webhooks and the webhook server.
type Webhook struct {
	// Server configures the sharder's webhook server.
	// +optional
	Server *WebhookServer `json:"server,omitempty"`
	// Config configures the sharder's MutatingWebhookConfiguration objects.
	// +optional
	Config *WebhookConfig `json:"config,omitempty"`
}

// WebhookServer configures the webhook server.
type WebhookServer struct {
	// CertDir is the directory that contains the server key and certificate.
	// Defaults to /tmp/k8s-webhook-server/serving-certs
	// +optional
	CertDir *string `json:"certDir,omitempty"`
	// CertName is the server certificate name.
	// Defaults to tls.crt
	// +optional
	CertName *string `json:"certName,omitempty"`
	// KeyName is the server key name.
	// Defaults to tls.key
	// +optional
	KeyName *string `json:"keyName,omitempty"`
}

// WebhookConfig configures the sharder's MutatingWebhookConfiguration objects.
type WebhookConfig struct {
	// Annotations are additional annotations that should be added to all webhook configs.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// ClientConfig configures the webhook configs' target.
	// Defaults to a service reference to sharding-system/sharder.
	// +optional
	ClientConfig *admissionregistrationv1.WebhookClientConfig `json:"clientConfig,omitempty"`
	// NamespaceSelector overwrites the webhook configs' default namespaceSelector.
	// Note: changing/unsetting this selector will not remove labels from objects in namespaces that were previously
	// included.
	// Defaults to excluding the kube-system and sharding-system namespaces
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}
