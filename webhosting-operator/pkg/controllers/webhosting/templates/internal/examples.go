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

package internal

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

// CreateExamples returns an example set of values for testing purposes.
func CreateExamples() (string, *webhostingv1alpha1.Website, *webhostingv1alpha1.Theme) {
	return "homepage-381fa2",
		&webhostingv1alpha1.Website{
			ObjectMeta: v1.ObjectMeta{
				Name:      "homepage",
				Namespace: "project-foo",
			},
			Spec: webhostingv1alpha1.WebsiteSpec{
				Theme: "fancy",
			},
		},
		&webhostingv1alpha1.Theme{
			ObjectMeta: v1.ObjectMeta{
				Name: "fancy",
			},
			Spec: webhostingv1alpha1.ThemeSpec{
				Color:      "darkcyan",
				FontFamily: "Menlo",
			},
		}
}
