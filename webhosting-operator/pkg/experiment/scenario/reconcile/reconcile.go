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

package reconcile

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
)

const ScenarioName = "reconcile"

var baseLog = logf.Log.WithName("scenario").WithName(ScenarioName)

func init() {
	experiment.RegisterScenario(&scenario{})
}

type scenario struct {
	done chan struct{}
	mgr  manager.Manager
	client.Client

	labels map[string]string
}

func (s *scenario) Name() string {
	return ScenarioName
}

func (s *scenario) Description() string {
	return "High frequency reconciliation load test scenario (15m) with a static number of websites (10k)"
}

func (s *scenario) LongDescription() string {
	return `The ` + ScenarioName + ` scenario generates 10k websites during preparation.
Then, it reconciles each website every 10s, i.e., it generates a reconciliation frequency of 1000/s.`
}

func (s *scenario) Done() <-chan struct{} {
	return s.done
}

func (s *scenario) AddToManager(mgr manager.Manager) error {
	s.done = make(chan struct{})
	s.mgr = mgr
	s.Client = mgr.GetClient()

	s.labels = map[string]string{
		"generated-by": "experiment",
		"scenario":     ScenarioName,
	}

	return mgr.Add(s)
}

func (s *scenario) Start(ctx context.Context) error {
	baseLog.Info("Scenario started")

	baseLog.Info("Creating owner object")
	ownerObject, ownerRef, err := generator.CreateClusterScopedOwnerObject(ctx, s.Client, generator.WithLabels(s.labels))
	if err != nil {
		return err
	}

	// use unique label set per scenario run
	s.labels["run-id"] = string(ownerObject.GetUID())
	log := baseLog.WithValues("runID", s.labels["run-id"])
	log.Info("Created owner object", "object", ownerObject)

	log.Info("Preparing themes")
	if err := generator.CreateThemes(ctx, s.Client, 50, generator.WithLabels(s.labels), generator.WithOwnerReference(ownerRef)); err != nil {
		return err
	}

	log.Info("Preparing projects")
	if err := generator.CreateProjects(ctx, s.Client, 20, generator.WithLabels(s.labels), generator.WithOwnerReference(ownerRef)); err != nil {
		return err
	}

	log.Info("Preparing websites")
	if err := generator.CreateWebsites(ctx, s.Client, 10000, generator.WithLabels(s.labels)); err != nil {
		return err
	}

	log.Info("Scenario prepared")

	// give monitoring stack some time to observe objects
	select {
	case <-ctx.Done():
		log.Info("Scenario cancelled")
		return ctx.Err()
	case <-time.After(30 * time.Second):
	}

	// trigger individual reconciliations for website every 10s
	if err := (&generator.ForEach[*webhostingv1alpha1.Website]{
		Name:      "website-reconcile-trigger",
		Do:        generator.ReconcileWebsite,
		Every:     10 * time.Second,
		RateLimit: rate.Limit(1000),
		Labels:    s.labels,
	}).AddToManager(s.mgr); err != nil {
		return fmt.Errorf("error adding website-reconcile-trigger: %w", err)
	}

	log.Info("Scenario running")

	select {
	case <-ctx.Done():
		log.Info("Scenario cancelled, cleaning up")
	case <-time.After(15 * time.Minute):
		log.Info("Scenario finished, cleaning up")
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Client.Delete(cleanupCtx, ownerObject, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return err
	}

	log.Info("Cleanup done")
	close(s.done)
	return nil
}
