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
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

func addDefaultingFuncs(scheme *runtime.Scheme) error {
	return RegisterDefaults(scheme)
}

func SetDefaults_SharderConfig(obj *SharderConfig) {
	if obj.ClientConnection == nil {
		obj.ClientConnection = &componentbaseconfigv1alpha1.ClientConnectionConfiguration{}
	}
	if obj.LeaderElection == nil {
		obj.LeaderElection = &componentbaseconfigv1alpha1.LeaderElectionConfiguration{}
	}
	if obj.Debugging == nil {
		obj.Debugging = &componentbaseconfigv1alpha1.DebuggingConfiguration{}
	}

	if obj.GracefulShutdownTimeout == nil {
		obj.GracefulShutdownTimeout = &metav1.Duration{Duration: 15 * time.Second}
	}
}

func SetDefaults_ClientConnectionConfiguration(obj *componentbaseconfigv1alpha1.ClientConnectionConfiguration) {
	// increase default rate limiter settings to make sharder and controller more responsive
	if obj.QPS == 0 {
		obj.QPS = 100
	}
	if obj.Burst == 0 {
		obj.Burst = 150
	}
}

func SetDefaults_LeaderElectionConfiguration(obj *componentbaseconfigv1alpha1.LeaderElectionConfiguration) {
	if obj.ResourceLock == "" {
		obj.ResourceLock = resourcelock.LeasesResourceLock
	}

	componentbaseconfigv1alpha1.RecommendedDefaultLeaderElectionConfiguration(obj)

	if obj.ResourceName == "" {
		obj.ResourceName = "sharder"
	}
	if obj.ResourceNamespace == "" {
		obj.ResourceNamespace = shardingv1alpha1.NamespaceSystem
	}
}

func SetDefaults_DebuggingConfiguration(obj *componentbaseconfigv1alpha1.DebuggingConfiguration) {
	componentbaseconfigv1alpha1.RecommendedDebuggingConfiguration(obj)

	if obj.EnableContentionProfiling == nil {
		obj.EnableContentionProfiling = ptr.To(false)
	}
}

func SetDefaults_HealthEndpoint(obj *HealthEndpoint) {
	if obj.BindAddress == "" {
		obj.BindAddress = ":8081"
	}
}

func SetDefaults_MetricsEndpoint(obj *MetricsEndpoint) {
	if obj.BindAddress == "" {
		obj.BindAddress = ":8080"
	}
}

func SetDefaults_Controller(obj *Controller) {
	if obj.Sharder == nil {
		obj.Sharder = &SharderController{}
	}
}

func SetDefaults_SharderController(obj *SharderController) {
	if obj.SyncPeriod == nil {
		obj.SyncPeriod = &metav1.Duration{Duration: 5 * time.Minute}
	}
}

func SetDefaults_Webhook(obj *Webhook) {
	if obj.Server == nil {
		obj.Server = &WebhookServer{}
	}

	if obj.Config == nil {
		obj.Config = &WebhookConfig{}
	}
}

func SetDefaults_WebhookConfig(obj *WebhookConfig) {
	if obj.ClientConfig == nil {
		obj.ClientConfig = &admissionregistrationv1.WebhookClientConfig{}
	}

	if obj.NamespaceSelector == nil {
		obj.NamespaceSelector = &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      corev1.LabelMetadataName,
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{metav1.NamespaceSystem, shardingv1alpha1.NamespaceSystem},
			}},
		}
	}
}

func SetDefaults_WebhookClientConfig(obj *admissionregistrationv1.WebhookClientConfig) {
	if obj.URL == nil && obj.Service == nil {
		obj.Service = &admissionregistrationv1.ServiceReference{}
	}
}

func SetDefaults_ServiceReference(obj *admissionregistrationv1.ServiceReference) {
	if obj.Namespace == "" {
		obj.Namespace = shardingv1alpha1.NamespaceSystem
	}
	if obj.Name == "" {
		obj.Name = "sharder"
	}
}
