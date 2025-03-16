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

package sharder_test

import (
	"context"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/webhook/sharder"
)

var _ = Describe("webhook path", func() {
	const controllerRingName = "foo"

	Describe("#WebhookPathForControllerRing", func() {
		It("should return the correct webhook path", func() {
			controllerRing := &shardingv1alpha1.ControllerRing{ObjectMeta: metav1.ObjectMeta{Name: controllerRingName}}

			Expect(WebhookPathForControllerRing(controllerRing)).To(Equal("/webhooks/sharder/controllerring/" + controllerRingName))
		})
	})

	Describe("#ControllerRingForWebhookPath", func() {
		It("should return an error for invalid paths", func() {
			matchError := MatchError(ContainSubstring("unexpected request path"))

			Expect(ControllerRingForWebhookPath("/foo")).Error().To(matchError)
			Expect(ControllerRingForWebhookPath("/webhooks")).Error().To(matchError)
			Expect(ControllerRingForWebhookPath("/webhooks/sharder")).Error().To(matchError)
			Expect(ControllerRingForWebhookPath("/webhooks/sharder/controllerring")).Error().To(matchError)
			Expect(ControllerRingForWebhookPath("/webhooks/sharder/controllerring/")).Error().To(matchError)
		})

		It("should return a ControllerRing with name", func() {
			Expect(ControllerRingForWebhookPath("/webhooks/sharder/controllerring/" + controllerRingName)).To(
				Equal(&shardingv1alpha1.ControllerRing{
					ObjectMeta: metav1.ObjectMeta{Name: controllerRingName},
				}),
			)
		})
	})
})

var _ = Describe("webhook context", func() {
	It("should inject and extract the request path into/from the context", func() {
		const path = "/webhooks/sharder/controllerring/foo"

		ctx := NewContextWithRequestPath(context.Background(), &http.Request{
			URL: &url.URL{Path: path},
		})

		Expect(RequestPathFromContext(ctx)).To(Equal(path))
	})

	It("should return an error if context doesn't contain path value", func() {
		Expect(RequestPathFromContext(context.Background())).Error().To(MatchError("no request path found in context"))
	})
})
