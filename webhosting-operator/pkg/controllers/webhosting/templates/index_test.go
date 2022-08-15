/*
Copyright 2022 Tim Ebert.

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

package templates_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting/templates"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting/templates/internal"
)

var _ = Describe("index.html", func() {
	It("should successfully render index page", func() {
		// just assert that the template can be rendered properly
		Expect(RenderIndexHTML(internal.CreateExamples())).To(ContainSubstring("Welcome to"))
	})
})
