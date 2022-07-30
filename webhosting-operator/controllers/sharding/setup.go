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

package sharding

import (
	"fmt"

	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Sharder struct {
	Clock clock.Clock

	Object         client.Object
	LeaseNamespace string
	TokensPerNode  int
}

func (s *Sharder) SetupWithManager(mgr manager.Manager) error {
	if s.Clock == nil {
		s.Clock = clock.RealClock{}
	}

	if err := (&leaseReconciler{
		Client: mgr.GetClient(),
		Clock:  s.Clock,

		LeaseNamespace: s.LeaseNamespace,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("error adding lease controller to manager: %w", err)
	}

	if err := (&shardingReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Clock:  s.Clock,

		Object:         s.Object,
		LeaseNamespace: s.LeaseNamespace,
		TokensPerNode:  s.TokensPerNode,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("error adding sharder to manager: %w", err)
	}

	return nil
}
