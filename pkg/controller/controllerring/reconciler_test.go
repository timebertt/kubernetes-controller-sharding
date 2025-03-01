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

package controllerring_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/controller/controllerring"
)

var _ = Describe("Reconciler", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		r          *Reconciler

		ring   *shardingv1alpha1.ControllerRing
		config *configv1alpha1.SharderConfig
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme := runtime.NewScheme()
		Expect(shardingv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(configv1alpha1.AddToScheme(scheme)).To(Succeed())

		ring = &shardingv1alpha1.ControllerRing{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "foo",
				Generation: 1,
			},
			Status: shardingv1alpha1.ControllerRingStatus{
				ObservedGeneration: 1,
				Shards:             0,
				AvailableShards:    0,
				Conditions: []metav1.Condition{{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				}},
			},
		}

		config = &configv1alpha1.SharderConfig{}
		scheme.Default(config)

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(ring).
			WithStatusSubresource(&shardingv1alpha1.ControllerRing{}).
			Build()

		r = &Reconciler{
			Client: fakeClient,
			Config: config,
		}
	})

	Describe("#OptionallyUpdateStatus", func() {
		It("should update status if observed generation is outdated", func() {
			ring.Generation++
			Expect(fakeClient.Update(ctx, ring)).To(Succeed())

			Expect(r.OptionallyUpdateStatus(ctx, ring, ring.DeepCopy(), func(ready *metav1.Condition) {})).Should(Succeed())

			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ring), ring)).Should(Succeed())
			Expect(ring.Status.ObservedGeneration).Should(Equal(ring.Generation))
			Expect(ring.Status.Conditions[0].ObservedGeneration).Should(Equal(ring.Generation))
		})

		It("should update status if condition mutated", func() {
			Expect(r.OptionallyUpdateStatus(ctx, ring, ring.DeepCopy(), func(ready *metav1.Condition) {
				ready.Status = metav1.ConditionFalse
			})).Should(Succeed())

			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ring), ring)).Should(Succeed())
			Expect(ring.Status.Conditions).Should(ConsistOf(MatchFields(IgnoreExtras, Fields{
				"Type":               Equal("Ready"),
				"Status":             Equal(metav1.ConditionFalse),
				"ObservedGeneration": Equal(ring.Generation),
			})))
		})

		It("should update status if mutated outside the function", func() {
			before := ring.DeepCopy()
			ring.Status.AvailableShards = 1

			Expect(r.OptionallyUpdateStatus(ctx, ring, before, func(ready *metav1.Condition) {})).Should(Succeed())

			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ring), ring)).Should(Succeed())
			Expect(ring.Status.AvailableShards).Should(BeEquivalentTo(1))
		})

		It("should skip updating status if nothing changed", func() {
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ring), ring)).Should(Succeed())
			resourceVersion := ring.ResourceVersion

			Expect(r.OptionallyUpdateStatus(ctx, ring, ring.DeepCopy(), func(ready *metav1.Condition) {
				*ready = ring.Status.Conditions[0]
			})).Should(Succeed())

			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(ring), ring)).Should(Succeed())
			Expect(ring.ResourceVersion).Should(Equal(resourceVersion))
		})
	})

	Describe("#WebhookConfigForControllerRing", func() {
		It("should have the correct metadata", func() {
			Expect(r.WebhookConfigForControllerRing(ring)).To(PointTo(MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Name": Equal("controllerring-" + ring.Name),
					"Labels": Equal(map[string]string{
						"app.kubernetes.io/name":                      "controller-sharding",
						"alpha.sharding.timebertt.dev/controllerring": ring.Name,
					}),
				}),
			})))
		})

		It("should copy the config's annotations", func() {
			config.Webhook.Config.Annotations = map[string]string{
				"my": "annotation",
			}

			Expect(r.WebhookConfigForControllerRing(ring)).To(PointTo(
				HaveField("ObjectMeta.Annotations", Equal(map[string]string{
					"my": "annotation",
				})),
			))
		})

		It("should set the controller reference", func() {
			ring.UID = "123456"

			Expect(r.WebhookConfigForControllerRing(ring)).To(PointTo(
				HaveField("ObjectMeta.OwnerReferences", ConsistOf(metav1.OwnerReference{
					APIVersion:         "sharding.timebertt.dev/v1alpha1",
					Kind:               "ControllerRing",
					Name:               ring.Name,
					UID:                ring.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				})),
			))
		})

		It("should have a single webhook", func() {
			Expect(r.WebhookConfigForControllerRing(ring)).To(HaveField("Webhooks", HaveLen(1)))
		})
	})

	Describe("#WebhookForControllerRing", func() {
		It("should have the correct settings", func() {
			Expect(WebhookForControllerRing(ring, config.Webhook.Config)).To(MatchFields(IgnoreExtras, Fields{
				"Name":                    Equal("sharder.sharding.timebertt.dev"),
				"SideEffects":             PointTo(Equal(admissionregistrationv1.SideEffectClassNone)),
				"AdmissionReviewVersions": ConsistOf("v1"),
			}))
		})

		It("should have non-problematic failure settings", func() {
			Expect(WebhookForControllerRing(ring, config.Webhook.Config)).To(MatchFields(IgnoreExtras, Fields{
				"FailurePolicy":  PointTo(Equal(admissionregistrationv1.Ignore)),
				"TimeoutSeconds": PointTo(BeEquivalentTo(5)),
			}))
		})

		Context("client config", func() {
			It("should use the config's default client config and add the path", func() {
				Expect(WebhookForControllerRing(ring, config.Webhook.Config).ClientConfig).To(Equal(admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "sharding-system",
						Name:      "sharder",
						Path:      ptr.To("/webhooks/sharder/controllerring/foo"),
					},
				}))
			})

			It("should use the service client config and add the path", func() {
				config.Webhook.Config.ClientConfig = &admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: "default",
						Name:      "webhook-service",
						Port:      ptr.To[int32](8080),
					},
				}
				clientConfig := config.Webhook.Config.ClientConfig.DeepCopy()

				Expect(WebhookForControllerRing(ring, config.Webhook.Config).ClientConfig).To(Equal(admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{
						Namespace: clientConfig.Service.Namespace,
						Name:      clientConfig.Service.Name,
						Port:      clientConfig.Service.Port,
						Path:      ptr.To("/webhooks/sharder/controllerring/foo"),
					},
				}))
			})

			It("should use the URL client config and add the path", func() {
				config.Webhook.Config.ClientConfig = &admissionregistrationv1.WebhookClientConfig{
					URL: ptr.To("https://example.com/webhook"),
				}

				Expect(WebhookForControllerRing(ring, config.Webhook.Config).ClientConfig).To(Equal(admissionregistrationv1.WebhookClientConfig{
					URL: ptr.To("https://example.com/webhook/webhooks/sharder/controllerring/foo"),
				}))
			})

			It("should use the URL client config with a trailing slash and add the path", func() {
				config.Webhook.Config.ClientConfig = &admissionregistrationv1.WebhookClientConfig{
					URL: ptr.To("https://example.com/"),
				}

				Expect(WebhookForControllerRing(ring, config.Webhook.Config).ClientConfig).To(Equal(admissionregistrationv1.WebhookClientConfig{
					URL: ptr.To("https://example.com/webhooks/sharder/controllerring/foo"),
				}))
			})
		})

		Context("namespace selector", func() {
			It("should use the config's default namespace selector", func() {
				Expect(WebhookForControllerRing(ring, config.Webhook.Config).NamespaceSelector).To(Equal(&metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key:      corev1.LabelMetadataName,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"kube-system", "sharding-system"},
					}},
				}))
			})

			It("should use the config's namespace selector", func() {
				config.Webhook.Config.NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"my": "label"},
				}
				namespaceSelector := config.Webhook.Config.NamespaceSelector.DeepCopy()

				Expect(WebhookForControllerRing(ring, config.Webhook.Config).NamespaceSelector).To(Equal(namespaceSelector))
			})

			It("should use the ControllerRing's namespace selector", func() {
				ring.Spec.NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{"my": "label"},
				}
				namespaceSelector := ring.Spec.NamespaceSelector.DeepCopy()

				Expect(WebhookForControllerRing(ring, config.Webhook.Config).NamespaceSelector).To(Equal(namespaceSelector))
			})
		})

		It("should only select unassigned objects", func() {
			selector, err := metav1.LabelSelectorAsSelector(WebhookForControllerRing(ring, config.Webhook.Config).ObjectSelector)
			Expect(err).NotTo(HaveOccurred())

			Expect(selector.Matches(labels.Set{})).To(BeTrue())
			Expect(selector.Matches(labels.Set{ring.LabelShard(): "shard-1"})).To(BeFalse())
		})

		It("should add rules for all resources", func() {
			ring.Spec.Resources = []shardingv1alpha1.RingResource{
				{
					GroupResource: metav1.GroupResource{Group: "", Resource: "configmaps"},
				},
				{
					GroupResource:       metav1.GroupResource{Group: "apps", Resource: "deployments"},
					ControlledResources: []metav1.GroupResource{{Group: "apps", Resource: "replicasets"}},
				},
			}

			Expect(WebhookForControllerRing(ring, config.Webhook.Config).Rules).To(ConsistOf(
				RuleForResource(ring.Spec.Resources[0].GroupResource),
				RuleForResource(ring.Spec.Resources[1].GroupResource),
				RuleForResource(ring.Spec.Resources[1].ControlledResources[0]),
			))
		})
	})

	Describe("#RuleForResource", func() {
		var gr metav1.GroupResource

		BeforeEach(func() {
			gr.Group = "apps"
			gr.Resource = "deployments"
		})

		It("should act on Create and Update", func() {
			Expect(RuleForResource(gr).Operations).To(ConsistOf(admissionregistrationv1.Create, admissionregistrationv1.Update))
		})

		It("should generate a matching rule", func() {
			Expect(RuleForResource(gr).Rule).To(Equal(admissionregistrationv1.Rule{
				APIGroups:   []string{"apps"},
				APIVersions: []string{"*"},
				Resources:   []string{"deployments"},
				Scope:       ptr.To(admissionregistrationv1.AllScopes),
			}))
		})
	})
})
