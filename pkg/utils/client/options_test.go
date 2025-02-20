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

package client_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/client"
)

var _ = Describe("ResourceVersion", func() {
	It("should set the resourceVersion on GetOptions", func() {
		opts := &client.GetOptions{}
		opts.ApplyOptions([]client.GetOption{ResourceVersion("1")})

		Expect(opts.Raw.ResourceVersion).To(Equal("1"))
	})

	It("should set the resourceVersion on ListOptions", func() {
		opts := &client.ListOptions{}
		opts.ApplyOptions([]client.ListOption{ResourceVersion("1")})

		Expect(opts.Raw.ResourceVersion).To(Equal("1"))
	})
})
