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
	"fmt"
	"reflect"

	"github.com/onsi/gomega/gcustom"
	gomegatypes "github.com/onsi/gomega/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// BeNotFoundError checks if error is a NotFound error.
func BeNotFoundError() gomegatypes.GomegaMatcher {
	return &errorMatcher{
		match:   apierrors.IsNotFound,
		message: "NotFound",
	}
}

// BeFunc is a matcher that returns true if expected and actual are the same func.
func BeFunc(expected any) gomegatypes.GomegaMatcher {
	return gcustom.MakeMatcher(func(actual any) (bool, error) {
		var (
			valueExpected = reflect.ValueOf(expected)
			valueActual   = reflect.ValueOf(actual)
		)

		if valueExpected.Kind() != reflect.Func {
			return false, fmt.Errorf("expected should be a func, got %v", valueExpected.Kind())
		}
		if valueActual.Kind() != reflect.Func {
			return false, fmt.Errorf("actual should be a func, got %v", valueActual.Kind())
		}

		return valueExpected.Pointer() == valueActual.Pointer(), nil
	})
}
