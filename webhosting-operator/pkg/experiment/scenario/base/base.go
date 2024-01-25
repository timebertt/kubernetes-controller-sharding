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

package base

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/tracker"
)

// Scenario provides a common base implemenation for parts of the experiment.Scenario interface.
type Scenario struct {
	// Delegate is responsible for performing the actual experiment actions.
	Delegate Delegate

	ScenarioName string

	Log     logr.Logger
	Manager manager.Manager
	client.Client
	done chan struct{}

	Labels   map[string]string
	OwnerRef *metav1.OwnerReference
}

// Delegate combines the actual experiment actions in a single interface.
type Delegate interface {
	Prepare(ctx context.Context) error
	Run(ctx context.Context) error
}

func (s *Scenario) Name() string {
	return s.ScenarioName
}

func (s *Scenario) Done() <-chan struct{} {
	return s.done
}

func (s *Scenario) AddToManager(mgr manager.Manager) error {
	s.Log = logf.Log.WithName("scenario").WithName(s.ScenarioName)
	s.Manager = mgr
	s.Client = mgr.GetClient()
	s.done = make(chan struct{})

	s.Labels = map[string]string{
		"generated-by": "experiment",
		"scenario":     s.ScenarioName,
	}

	return mgr.Add(s)
}

func (s *Scenario) Start(ctx context.Context) (err error) {
	defer close(s.done)
	s.Log.Info("Scenario started")

	cleanup, err := s.prepare(ctx)
	if err != nil {
		return err
	}

	defer func() {
		s.Log.Info("Cleaning up")
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if cleanupErr := cleanup(cleanupCtx); cleanupErr != nil {
			// if another error occurred during execution, it has priority
			// otherwise, return the cleanup error
			if err != nil {
				s.Log.Error(cleanupErr, "Failed cleaning up")
			} else {
				err = cleanupErr
			}

			return
		}

		s.Log.Info("Cleanup done")
	}()

	if err := s.Delegate.Prepare(ctx); err != nil {
		return err
	}

	s.Log.Info("Scenario prepared")

	// give monitoring stack some time to observe objects
	select {
	case <-ctx.Done():
		s.Log.Info("Scenario cancelled")
		return ctx.Err()
	case <-time.After(30 * time.Second):
	}

	websiteTracker := &tracker.WebsiteTracker{}
	if err := websiteTracker.AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-tracker: %w", err)
	}
	generator.SetWebsiteTracker(websiteTracker)

	if err := s.Delegate.Run(ctx); err != nil {
		return err
	}

	s.Log.Info("Scenario running")

	select {
	case <-ctx.Done():
		s.Log.Info("Scenario cancelled")
		return ctx.Err()
	case <-time.After(15 * time.Minute):
	}

	s.Log.Info("Scenario finished")
	return nil
}

func (s *Scenario) prepare(ctx context.Context) (func(context.Context) error, error) {
	s.Log.Info("Creating owner object")
	ownerObject, ownerRef, err := generator.CreateClusterScopedOwnerObject(ctx, s.Client, generator.WithLabels(s.Labels))
	if err != nil {
		return nil, err
	}
	s.OwnerRef = ownerRef

	// use unique label set per scenario run
	s.Labels["run-id"] = string(ownerObject.GetUID())
	s.Log = s.Log.WithValues("runID", s.Labels["run-id"])
	s.Log.Info("Created owner object", "object", ownerObject)

	cleanup := func(cleanupCtx context.Context) error {
		if err := s.Client.Delete(cleanupCtx, ownerObject, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			return fmt.Errorf("failed cleaning up owner object %T %s", ownerObject, client.ObjectKeyFromObject(ownerObject))
		}
		return nil
	}

	s.Log.Info("Restarting observed components")

	// restart observed components to start from a fresh state
	if err := s.Client.DeleteAllOf(ctx, &corev1.Pod{},
		client.InNamespace("sharding-system"), client.MatchingLabels{"app.kubernetes.io/component": "sharder"},
	); err != nil {
		return nil, err
	}
	if err := s.Client.DeleteAllOf(ctx, &corev1.Pod{},
		client.InNamespace("webhosting-system"), client.MatchingLabels{"app.kubernetes.io/name": "webhosting-operator"},
	); err != nil {
		return nil, err
	}

	// clean up orphaned leases after instances have been terminated
	select {
	case <-ctx.Done():
		s.Log.Info("Scenario cancelled")
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
	}

	if err := s.Client.DeleteAllOf(ctx, &coordinationv1.Lease{},
		client.InNamespace("webhosting-system"), client.MatchingLabels{"alpha.sharding.timebertt.dev/state": "dead"},
	); err != nil {
		return nil, err
	}

	// wait for all shard leases to be ready
	if err := s.waitForShardLeases(ctx); err != nil {
		return nil, fmt.Errorf("failed waiting for shard leases: %w", err)
	}

	return cleanup, nil
}

func (s *Scenario) waitForShardLeases(ctx context.Context) error {
	var lastError error
	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, true, func(ctx context.Context) (done bool, err error) {
		leaseList := &coordinationv1.LeaseList{}
		if err := s.Client.List(ctx, leaseList,
			client.InNamespace("webhosting-system"), client.MatchingLabels{"alpha.sharding.timebertt.dev/clusterring": "webhosting-operator"},
		); err != nil {
			return true, err
		}

		for _, lease := range leaseList.Items {
			state := lease.Labels["alpha.sharding.timebertt.dev/state"]
			if state != "ready" {
				lastError = fmt.Errorf("shard lease %s is in state %q", client.ObjectKeyFromObject(&lease), state)
				return false, nil
			}
		}

		return true, nil
	}); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return lastError
		}
		return err
	}
	return nil
}
