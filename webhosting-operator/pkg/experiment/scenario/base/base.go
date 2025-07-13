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
	"os"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/tracker"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils"
)

// Scenario provides a common base implemenation for parts of the experiment.Scenario interface.
type Scenario struct {
	// Delegate is responsible for performing the actual experiment actions.
	Delegate Delegate

	ScenarioName string

	Log     logr.Logger
	Manager manager.Manager
	Client  client.Client
	done    chan struct{}

	RunID    string
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

		webhostingv1alpha1.LabelKeySkipWorkload: "true",
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
		s.Log.Info("Scenario canceled")
		return ctx.Err()
	case <-time.After(30 * time.Second):
	}

	websiteTracker := &tracker.WebsiteTracker{
		RunID: s.RunID,
	}
	if err := websiteTracker.AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-tracker: %w", err)
	}
	generator.SetWebsiteTracker(websiteTracker)

	s.Log.Info("Scenario running")

	if err := s.Delegate.Run(ctx); err != nil {
		return err
	}

	s.Log.Info("Scenario finished")
	return nil
}

func (s *Scenario) prepare(ctx context.Context) (func(context.Context) error, error) {
	// determine run ID
	// env var can be specified in the pod spec, e.g. from the pod's uid so that experiment's own metrics can be selected
	// by run ID
	s.RunID = os.Getenv("RUN_ID")
	if s.RunID == "" {
		s.RunID = string(uuid.NewUUID())
	}

	// use unique label set per scenario run
	s.Labels["run-id"] = s.RunID
	s.Log = s.Log.WithValues("run-id", s.RunID)

	s.Log.Info("Creating owner object")
	ownerObject, ownerRef, err := generator.CreateClusterScopedOwnerObject(ctx, s.Client, generator.WithLabels(s.Labels))
	if err != nil {
		return nil, err
	}
	s.OwnerRef = ownerRef
	s.Log.Info("Created owner object", "object", ownerObject)

	cleanup := func(cleanupCtx context.Context) error {
		if err := s.Client.Delete(cleanupCtx, ownerObject, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			return fmt.Errorf("failed cleaning up owner object %T %s", ownerObject, client.ObjectKeyFromObject(ownerObject))
		}
		return nil
	}

	// Label all observed components with the experiment's run ID to trigger a rollout (start from a fresh state).
	// The run ID is added to all scraped metrics in the ServiceMonitor.
	// This allows calculating rates of individual runs without considering metrics of adjacent runs.
	s.Log.Info("Restarting/labeling observed components")
	observedComponents := []struct{ namespace, name string }{
		{shardingv1alpha1.NamespaceSystem, "sharder"},
		{webhostingv1alpha1.NamespaceSystem, webhostingv1alpha1.WebhostingOperatorName},
	}

	for _, c := range observedComponents {
		if err := s.injectRunIDLabel(ctx, c.namespace, c.name); err != nil {
			return nil, err
		}
	}

	// wait for all observed components to be rolled out and ready again
	for _, c := range observedComponents {
		if err := s.waitForDeployment(ctx, c.namespace, c.name); err != nil {
			return nil, fmt.Errorf("failed waiting for deployment: %w", err)
		}
	}

	// clean up orphaned leases after instances have been terminated
	// this only speeds up the cleaning and allows us to start sooner, but is not required otherwise
	if err := s.Client.DeleteAllOf(ctx, &coordinationv1.Lease{},
		client.InNamespace(webhostingv1alpha1.NamespaceSystem), client.MatchingLabels{"alpha.sharding.timebertt.dev/state": "dead"},
	); err != nil {
		return nil, err
	}

	// wait for all shard leases to be ready
	if err := s.waitForShardLeases(ctx); err != nil {
		return nil, fmt.Errorf("failed waiting for shard leases: %w", err)
	}

	return cleanup, nil
}

func (s *Scenario) injectRunIDLabel(ctx context.Context, namespace, name string) error {
	deployment := &appsv1.Deployment{}
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, deployment); err != nil {
		return err
	}

	patch := client.MergeFrom(deployment.DeepCopy())
	metav1.SetMetaDataLabel(&deployment.Spec.Template.ObjectMeta, "label.prometheus.io/run_id", s.RunID)
	return s.Client.Patch(ctx, deployment, patch)
}

func (s *Scenario) waitForDeployment(ctx context.Context, namespace, name string) error {
	var lastError error
	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, false, func(ctx context.Context) (done bool, err error) {
		deployment := &appsv1.Deployment{}
		if err := s.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, deployment); err != nil {
			return true, err
		}

		if !utils.IsDeploymentReady(deployment) {
			lastError = fmt.Errorf("deployment %s is not available", client.ObjectKeyFromObject(deployment))
			return false, nil
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

func (s *Scenario) waitForShardLeases(ctx context.Context) error {
	var lastError error
	if err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 2*time.Minute, false, func(ctx context.Context) (done bool, err error) {
		leaseList := &coordinationv1.LeaseList{}
		if err := s.Client.List(ctx, leaseList,
			client.InNamespace(webhostingv1alpha1.NamespaceSystem), client.MatchingLabels{shardingv1alpha1.LabelControllerRing: webhostingv1alpha1.WebhostingOperatorName},
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

// Wait does blocks until the given duration has passed or returns an error when the context is canceled.
func (s *Scenario) Wait(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		s.Log.Info("Scenario canceled")
		return ctx.Err()
	case <-time.After(d):
	}
	return nil
}
