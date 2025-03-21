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

package matchers

import (
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
)

// HaveName succeeds if the actual object has a matching name.
func HaveName(name string) gomegatypes.GomegaMatcher {
	return HaveField("ObjectMeta.Name", name)
}

// HaveNames succeeds if the actual list consists of matching names.
func HaveNames(names ...string) gomegatypes.GomegaMatcher {
	matchers := make([]any, len(names))
	for i, n := range names {
		matchers[i] = HaveName(n)
	}

	return HaveField("Items", ConsistOf(matchers...))
}

// HaveLabel succeeds if the actual object has a label with a matching key.
func HaveLabel(key any) gomegatypes.GomegaMatcher {
	return HaveField("ObjectMeta.Labels", HaveKey(key))
}

// HaveLabelWithValue succeeds if the actual object has a label with a matching key and value.
func HaveLabelWithValue(key, value any) gomegatypes.GomegaMatcher {
	return HaveField("ObjectMeta.Labels", HaveKeyWithValue(key, value))
}
