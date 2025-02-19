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

package consistenthash_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/consistenthash"
)

var _ = Describe("Ring", func() {
	Describe("#New", func() {
		It("should initialize a new Ring", func() {
			ring := New(nil, 0, "foo")
			Expect(ring).NotTo(BeNil())
			Expect(ring.IsEmpty()).To(BeFalse())
		})
	})

	Describe("#IsEmpty", func() {
		It("should true if there are no nodes", func() {
			ring := New(nil, 0)
			Expect(ring.IsEmpty()).To(BeTrue())
			ring.AddNodes("foo")
			Expect(ring.IsEmpty()).To(BeFalse())
		})
	})

	Describe("#Hash", func() {
		It("should use the configured hash function", func() {
			ring := New(func(data string) uint64 {
				if strings.HasPrefix(data, "foo") {
					// map all foo* nodes and keys to 1
					return 1
				}
				return 2
			}, 1, "foo", "bar")

			Expect(ring.Hash("foo")).To(Equal("foo"))
			Expect(ring.Hash("bar")).To(Equal("bar"))
			Expect(ring.Hash("baz")).To(Equal("bar"))
		})

		It("should use the default hash function", func() {
			ring := New(nil, 0, "foo", "bar")

			Expect(ring.Hash("1")).NotTo(Equal(ring.Hash("10")))
		})

		It("should return the empty string if there are no nodes", func() {
			ring := New(nil, 0)

			Expect(ring.Hash("foo")).To(BeEmpty())
		})

		It("should return the first node when walking the whole ring", func() {
			ring := New(func(data string) uint64 {
				if strings.HasPrefix(data, "foo") {
					// map all foo* nodes and keys to 1
					return 1
				}
				if strings.HasPrefix(data, "bar") {
					// map all bar* nodes and keys to 1
					return 2
				}
				return 3
			}, 1, "foo", "bar")

			Expect(ring.Hash("foo")).To(Equal("foo"))
			Expect(ring.Hash("bar")).To(Equal("bar"))
			Expect(ring.Hash("baz")).To(Equal("foo"))
		})
	})
})
