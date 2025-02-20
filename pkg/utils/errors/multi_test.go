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

package errors_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/errors"
)

var _ = Describe("FormatErrors", func() {
	It("should return the single error", func() {
		Expect(FormatErrors([]error{fmt.Errorf("foo")})).To(Equal("foo"))
	})

	It("should return the error count and comma separated error list", func() {
		Expect(
			FormatErrors([]error{fmt.Errorf("foo"), fmt.Errorf("bar")}),
		).To(
			Equal("2 errors occurred: foo, bar"),
		)
	})
})
