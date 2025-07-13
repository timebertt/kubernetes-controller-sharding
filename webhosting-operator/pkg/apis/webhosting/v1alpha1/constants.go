/*
Copyright 2023 Tim Ebert.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// NamespaceSystem is the namespace where the webhosting-operator runs.
	NamespaceSystem = "webhosting-system"
	// WebhostingOperatorName is the name of the webhosting-operator Deployment, ControllerRing, etc.
	WebhostingOperatorName = "webhosting-operator"
	// LabelKeyProject is the label key on Namespaces for identifying webhosting projects.
	LabelKeyProject = "webhosting.timebertt.dev/project"
	// LabelValueProject is the label value for LabelKeyProject on Namespaces for identifying webhosting projects.
	LabelValueProject = "true"

	// LabelKeySkipWorkload is the label key on Websites for instructing webhosting-operator to skip any actual workload
	// for the website. Any value is accepted as truthy.
	LabelKeySkipWorkload = "skip-workload"
)

var (
	// LabelSelectorProject is a selector for webhosting project namespaces.
	LabelSelectorProject = labels.SelectorFromSet(labels.Set{LabelKeyProject: LabelValueProject})
)
