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

package metrics

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const namespace = "kube"

// AddToManager adds all metrics exporters for webhosting objects to the manager.
func AddToManager(mgr manager.Manager) error {
	if err := WebsiteExporter.AddToManager(mgr); err != nil {
		return fmt.Errorf("failed to add website exporter: %w", err)
	}

	if err := ThemeExporter.AddToManager(mgr); err != nil {
		return fmt.Errorf("failed to add theme exporter: %w", err)
	}

	return nil
}
