/*
Copyright 2024 Tim Ebert.

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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

var _ = Describe("Default Test", Label("default"), Ordered, func() {
	It("Should find the example ClusterRing", func(ctx SpecContext) {
		clusterRing := &shardingv1alpha1.ClusterRing{ObjectMeta: metav1.ObjectMeta{Name: "example"}}
		Eventually(ctx, Get(clusterRing)).WithTimeout(ShortTimeout).Should(Succeed())
		Expect(clusterRing.Status.AvailableShards).To(BeEquivalentTo(3))
	})
})
