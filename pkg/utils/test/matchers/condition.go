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
	"github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MatchCondition is an alias for gomega.And to make matching conditions more readable, e.g.,
//
//	Expect(controllerRing.Status.Conditions).To(ConsistOf(
//		MatchCondition(
//			OfType(shardingv1alpha1.ControllerRingReady),
//			WithStatus(metav1.ConditionTrue),
//		),
//	))
var MatchCondition = And

// OfType returns a matcher for checking whether a condition has a certain type.
func OfType(conditionType string) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Type": Equal(conditionType),
	})
}

// WithStatus returns a matcher for checking whether a condition has a certain status.
func WithStatus(status metav1.ConditionStatus) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Status": Equal(status),
	})
}

// WithReason returns a matcher for checking whether a condition has a certain reason.
func WithReason(reason string) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Reason": Equal(reason),
	})
}

// WithMessage returns a matcher for checking whether a condition has a certain message.
func WithMessage(message string) gomegatypes.GomegaMatcher {
	return gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Message": ContainSubstring(message),
	})
}
