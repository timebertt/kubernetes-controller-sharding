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

package base

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
)

const ScenarioName = "base"

var log = logf.Log

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
	log.Info("Scenario started")

	// generate themes
	for i := 0; i < 50; i++ {
		if err := generator.CreateTheme(ctx, s.Client, s.labels); err != nil {
			return err
		}
	}

	// generate projects
	for i := 0; i < 20; i++ {
		if err := generator.CreateProject(ctx, s.Client, s.labels); err != nil {
			return err
		}
	}

	log.Info("Scenario prepared")

	// website-generator: creates about 1200 websites over  10 minutes
	// website-deleter:   deletes about  100 websites over  10 minutes
	// => in total, there will be about 1100 websites after 10 minutes
	if err := (&generator.Every{
		Name:   "website-generator",
		Do:     generator.CreateWebsite,
		Every:  500 * time.Millisecond,
		Stop:   time.Now().Add(10 * time.Minute),
		Labels: s.labels,
	}).AddToManager(s.mgr); err != nil {
		return fmt.Errorf("error adding website-generator: %w", err)
	}

	if err := (&generator.Every{
		Name:   "website-deleter",
		Do:     generator.DeleteWebsite,
		Every:  6 * time.Second,
		Stop:   time.Now().Add(10 * time.Minute),
		Labels: s.labels,
	}).AddToManager(s.mgr); err != nil {
		return fmt.Errorf("error adding website-deleter: %w", err)
	}

	// individually trigger reconciliation for each website twice per minute
	// => peeks at about 37 reconciliations per second on average
	if err := (&generator.ForEach[*webhostingv1alpha1.Website]{
		Name:   "website-mutator",
		Do:     generator.ReconcileWebsite,
		Every:  30 * time.Second,
		Labels: s.labels,
	}).AddToManager(s.mgr); err != nil {
		return fmt.Errorf("error adding website-mutator: %w", err)
	}

	// update one theme every 15 seconds which causes all referencing websites to be reconciled
	// => peeks at about 1.5 reconciliations per second on average
	// (note: these reconciliation triggers occur in bursts of up to ~20)
	if err := (&generator.Every{
		Name:   "theme-mutator",
		Do:     generator.MutateTheme,
		Every:  15 * time.Second,
		Labels: s.labels,
	}).AddToManager(s.mgr); err != nil {
		return fmt.Errorf("error adding theme-mutator: %w", err)
	}

	log.Info("Scenario running")

	select {
	case <-ctx.Done():
		log.Info("Scenario cancelled")
		return ctx.Err()
	case <-time.After(15 * time.Minute):
	}

	log.Info("Scenario finished, cleaning up")
	close(s.done)

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := generator.CleanupProjects(cleanupCtx, s.Client, s.labels); err != nil {
		return err
	}
	if err := generator.CleanupThemes(cleanupCtx, s.Client, s.labels); err != nil {
		return err
	}

	log.Info("Cleanup done")
	return nil
}
