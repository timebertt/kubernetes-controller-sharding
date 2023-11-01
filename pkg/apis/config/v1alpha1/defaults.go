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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"
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
		var defaultGracefulShutdownTimeout = 15 * time.Second
		obj.GracefulShutdownTimeout = &metav1.Duration{Duration: defaultGracefulShutdownTimeout}
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
		obj.ResourceNamespace = "sharding-system"
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
