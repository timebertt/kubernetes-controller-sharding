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

// Package komega is a modified version of sigs.k8s.io/controller-runtime/pkg/envtest/komega.
// Instead of requiring users to set a global context, the returned functions accept a context to integrate nicely with
// ginkgo's/gomega's timeout/interrupt handling and polling.
// For this, users must pass a spec-specific context, e.g.:
//
//	It("...", func(ctx SpecContext) {
//		Eventually(ctx, Get(...)).Should(Succeed())
//	})
package komega
