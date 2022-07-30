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
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/sharding/cache"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/consistenthash"
)

const (
	defaultLeaseDuration = 15 * time.Second
	leaseTTL             = time.Minute
)

type Sharder struct {
	Reader         client.Reader
	Object         client.Object
	LeaseNamespace string
	Clock          clock.Clock

	ring               *consistenthash.Ring
	actualStateOfWorld *cache.ActualStateOfWorld
}

func (s *Sharder) SetupWithManager(ctx context.Context, mgr manager.Manager) error {
	if s.Reader == nil {
		// use API reader for populating actual state of world, cache is not started yet
		s.Reader = mgr.GetAPIReader()
	}
	if s.Clock == nil {
		s.Clock = clock.RealClock{}
	}

	s.ring = consistenthash.New(consistenthash.DefaultHash, consistenthash.DefaultTokensPerNode)
	s.actualStateOfWorld = &cache.ActualStateOfWorld{
		Clock: s.Clock,
	}

	// need to populate state of world before controllers start reconciling
	if err := s.populateActualStateOfWorld(ctx, mgr.GetLogger().WithName("sharder")); err != nil {
		return fmt.Errorf("error populating actual state of world: %w", err)
	}

	if err := (&leaseReconciler{
		Client: mgr.GetClient(),
		Clock:  s.Clock,

		LeaseNamespace: s.LeaseNamespace,

		Ring: s.ring,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("error adding lease controller to manager: %w", err)
	}

	if err := (&shardingReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Clock:  s.Clock,

		Object:         s.Object,
		LeaseNamespace: s.LeaseNamespace,

		Ring: s.ring,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("error adding sharder to manager: %w", err)
	}

	return nil
}

func (s *Sharder) populateActualStateOfWorld(ctx context.Context, log logr.Logger) error {
	leaseList := &coordinationv1.LeaseList{}
	// TODO: add labels to shard leases
	if err := s.Reader.List(ctx, leaseList, client.InNamespace(s.LeaseNamespace)); err != nil {
		return fmt.Errorf("error listing shard leasees: %w", err)
	}

	s.actualStateOfWorld.SetLeases(leaseList.Items)

	for _, shard := range s.actualStateOfWorld.GetReadyShards() {
		log.V(1).Info("Adding ready shard to ring", "shardID", shard.ID)
		s.ring.AddNode(shard.ID)
	}

	log.Info("Successfully populated ActualStateOfWorld")
	return nil
}
