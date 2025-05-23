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

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

var _ = Describe("#LabelShard", func() {
	It("should append the ControllerRing name", func() {
		Expect(LabelShard("foo")).To(Equal("shard.alpha.sharding.timebertt.dev/foo"))
	})
})

var _ = Describe("#LabelDrain", func() {
	It("should append the ControllerRing name", func() {
		Expect(LabelDrain("foo")).To(Equal("drain.alpha.sharding.timebertt.dev/foo"))
	})
})
