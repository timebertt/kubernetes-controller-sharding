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

package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/controller/clusterring"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/controller/sharder"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/controller/shardlease"
)

// AddToManager adds all controllers to the manager.
func AddToManager(ctx context.Context, mgr manager.Manager, config *configv1alpha1.SharderConfig) error {
	if err := (&clusterring.Reconciler{
		Config: config,
	}).AddToManager(mgr); err != nil {
		return fmt.Errorf("failed adding clusterring controller: %w", err)
	}

	if err := (&sharder.Reconciler{
		Config: config,
	}).AddToManager(mgr); err != nil {
		return fmt.Errorf("failed adding sharder controller: %w", err)
	}

	if err := (&shardlease.Reconciler{}).AddToManager(mgr); err != nil {
		return fmt.Errorf("failed adding lease controller: %w", err)
	}

	return nil
}
