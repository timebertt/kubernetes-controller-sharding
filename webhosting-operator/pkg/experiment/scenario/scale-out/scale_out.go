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

package scaleout

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/scenario/base"
)

const ScenarioName = "scale-out"

func init() {
	s := &scenario{}
	s.Scenario = &base.Scenario{
		ScenarioName: ScenarioName,
		Delegate:     s,
	}

	experiment.RegisterScenario(s)
}

type scenario struct {
	*base.Scenario
}

func (s *scenario) Description() string {
	return "Measure scale-out properties with a high churn rate"
}

func (s *scenario) LongDescription() string {
	return `The ` + ScenarioName + ` scenario is designed to overload 5 webhosting-operator instances with 5 workers within 15m:
- website creation: 9000 over 15m
- website spec changes: 2/m per object, max 300/s
`
}

func (s *scenario) Prepare(ctx context.Context) error {
	s.Log.Info("Preparing themes")
	if err := generator.CreateThemes(ctx, s.Client, 50, generator.WithLabels(s.Labels), generator.WithOwnerReference(s.OwnerRef)); err != nil {
		return err
	}

	s.Log.Info("Preparing projects")
	if err := generator.CreateProjects(ctx, s.Client, 20, generator.WithLabels(s.Labels), generator.WithOwnerReference(s.OwnerRef)); err != nil {
		return err
	}

	return nil
}

func (s *scenario) Run(ctx context.Context) error {
	// website-generator: creates about 9000 websites over 15 minutes
	if err := (&generator.Every{
		Name: "website-generator",
		Do: func(ctx context.Context, c client.Client) error {
			return generator.CreateWebsite(ctx, c, generator.WithLabels(s.Labels))
		},
		Rate: rate.Limit(10),
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-generator: %w", err)
	}

	// trigger individual spec changes for website twice per minute
	// => peaks at about 300 spec changes per second at the end of the experiment
	// (triggers roughly double the reconciliation rate in website controller because of deployment watches)
	if err := (&generator.ForEach[*webhostingv1alpha1.Website]{
		Name: "website-mutator",
		Do: func(ctx context.Context, c client.Client, obj *webhostingv1alpha1.Website) error {
			return client.IgnoreNotFound(generator.MutateWebsite(ctx, c, obj, s.Labels))
		},
		Every:   30 * time.Second,
		Workers: 20,
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-mutator: %w", err)
	}

	return s.Wait(ctx, 15*time.Minute)
}
