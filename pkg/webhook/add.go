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

package webhook

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/ring"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/webhook/sharder"
)

// AddToManager adds all webhooks to the manager.
func AddToManager(ctx context.Context, mgr manager.Manager, ringCache ring.Cache, config *configv1alpha1.SharderConfig) error {
	if err := (&sharder.Handler{
		Cache: ringCache,
	}).AddToManager(mgr); err != nil {
		return fmt.Errorf("failed adding sharder webhook: %w", err)
	}

	return nil
}
