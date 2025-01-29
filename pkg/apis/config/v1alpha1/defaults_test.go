/*
Copyright 2025 Tim Ebert.

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

package v1alpha1_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/ptr"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
)

var _ = Describe("SharderConfig defaulting", func() {
	var obj *SharderConfig

	BeforeEach(func() {
		obj = &SharderConfig{}
	})

	Context("ClientConnectionConfiguration", func() {
		It("should set default values", func() {
			SetObjectDefaults_SharderConfig(obj)

			Expect(obj.ClientConnection).To(Equal(&componentbaseconfigv1alpha1.ClientConnectionConfiguration{
				QPS:   100,
				Burst: 150,
			}))
		})
	})

	Context("LeaderElectionConfiguration", func() {
		It("should set default values", func() {
			SetObjectDefaults_SharderConfig(obj)

			Expect(obj.LeaderElection).To(Equal(&componentbaseconfigv1alpha1.LeaderElectionConfiguration{
				LeaderElect:       ptr.To(true),
				LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
				RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
				RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
				ResourceLock:      "leases",
				ResourceName:      "sharder",
				ResourceNamespace: "sharding-system",
			}))
		})
	})

	Context("DebuggingConfiguration", func() {
		It("should set default values", func() {
			SetObjectDefaults_SharderConfig(obj)

			Expect(obj.Debugging).To(Equal(&componentbaseconfigv1alpha1.DebuggingConfiguration{
				EnableProfiling:           ptr.To(true),
				EnableContentionProfiling: ptr.To(false),
			}))
		})
	})

	Context("manager settings", func() {
		It("should set default values", func() {
			SetObjectDefaults_SharderConfig(obj)

			Expect(obj.Health).To(Equal(HealthEndpoint{
				BindAddress: ":8081",
			}))
			Expect(obj.Metrics).To(Equal(MetricsEndpoint{
				BindAddress: ":8080",
			}))
			Expect(obj.GracefulShutdownTimeout).To(Equal(&metav1.Duration{Duration: 15 * time.Second}))
		})
	})

	Context("controller config", func() {
		It("should set default values", func() {
			SetObjectDefaults_SharderConfig(obj)

			Expect(obj.Controller).To(Equal(Controller{
				Sharder: &SharderController{
					SyncPeriod: &metav1.Duration{Duration: 5 * time.Minute},
				},
			}))
		})
	})

	Context("webhook config", func() {
		It("should set default values", func() {
			SetObjectDefaults_SharderConfig(obj)

			Expect(obj.Webhook).To(Equal(Webhook{
				Server: &WebhookServer{},
				Config: &WebhookConfig{
					ClientConfig: &admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{
							Namespace: "sharding-system",
							Name:      "sharder",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key:      "kubernetes.io/metadata.name",
							Operator: "NotIn",
							Values:   []string{"kube-system", "sharding-system"},
						}},
					},
				},
			}))
		})
	})
})
