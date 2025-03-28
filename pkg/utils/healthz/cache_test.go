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

package healthz_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/healthz"
)

var _ = Describe("CacheSync", func() {
	It("should succeed if all informers sync", func() {
		checker := CacheSync(fakeSyncWaiter(true))
		Expect(checker(nil)).NotTo(HaveOccurred())
	})
	It("should fail if informers don't sync", func() {
		checker := CacheSync(fakeSyncWaiter(false))
		Expect(checker(nil)).To(MatchError(ContainSubstring("not synced")))
	})
})

type fakeSyncWaiter bool

func (f fakeSyncWaiter) WaitForCacheSync(_ context.Context) bool {
	return bool(f)
}
