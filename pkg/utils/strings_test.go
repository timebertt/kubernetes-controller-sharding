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

package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils"
)

var _ = Describe("CapitalizeFirst", func() {
	It("should capitalize the first letter", func() {
		Expect(CapitalizeFirst("foo bar Baz")).To(Equal("Foo bar Baz"))
		Expect(CapitalizeFirst("Foo BAR Baz")).To(Equal("Foo BAR Baz"))
		Expect(CapitalizeFirst("FOO bar Baz")).To(Equal("FOO bar Baz"))
	})
})
