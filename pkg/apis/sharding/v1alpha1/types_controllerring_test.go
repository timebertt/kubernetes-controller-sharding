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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

var _ = Describe("ControllerRing", func() {
	var ring *ControllerRing

	BeforeEach(func() {
		ring = &ControllerRing{
			ObjectMeta: metav1.ObjectMeta{
				Name: "operator",
			},
			Spec: ControllerRingSpec{
				Resources: []RingResource{{
					GroupResource: metav1.GroupResource{
						Group:    "",
						Resource: "configmaps",
					},
					ControlledResources: []metav1.GroupResource{{
						Group:    "",
						Resource: "secrets",
					}},
				}},
			},
		}
	})

	Describe("#LeaseSelector", func() {
		It("should return the correct lease selector", func() {
			Expect(ring.LeaseSelector().String()).To(Equal("alpha.sharding.timebertt.dev/controllerring=operator"))
		})
	})

	Describe("#LabelShard", func() {
		It("should append the ControllerRing name", func() {
			Expect(ring.LabelShard()).To(Equal("shard.alpha.sharding.timebertt.dev/operator"))
		})
	})

	Describe("#LabelDrain", func() {
		It("should append the ControllerRing name", func() {
			Expect(ring.LabelDrain()).To(Equal("drain.alpha.sharding.timebertt.dev/operator"))
		})
	})
})
