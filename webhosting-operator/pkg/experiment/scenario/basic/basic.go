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

package basic

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

const ScenarioName = "basic"

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
	return "Basic load test scenario that creates roughly 10k websites over 15m"
}

func (s *scenario) LongDescription() string {
	return `The ` + ScenarioName + ` scenario combines several operations typical for a lively operator environment:
- website creation: 10800 over 15m
- website deletion: 900 over 15m
- website spec changes: 1/m per object, max 165/s
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
	// website-generator: creates about 10800 websites over  15 minutes
	// website-deleter:   deletes about   900 websites over  15 minutes
	// => in total, there will be about  9900 websites after 15 minutes
	if err := (&generator.Every{
		Name: "website-generator",
		Do: func(ctx context.Context, c client.Client) error {
			return generator.CreateWebsite(ctx, c, generator.WithLabels(s.Labels))
		},
		Rate: rate.Limit(12),
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-generator: %w", err)
	}

	if err := (&generator.Every{
		Name: "website-deleter",
		Do: func(ctx context.Context, c client.Client) error {
			return generator.DeleteWebsite(ctx, c, s.Labels)
		},
		Rate: rate.Limit(2),
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-deleter: %w", err)
	}

	// trigger individual spec changes for website once per minute
	// => peaks at about 165 spec changes per second at the end of the experiment
	// (triggers roughly double the reconciliation rate in website controller because of deployment watches)
	if err := (&generator.ForEach[*webhostingv1alpha1.Website]{
		Name: "website-mutator",
		Do: func(ctx context.Context, c client.Client, obj *webhostingv1alpha1.Website) error {
			return client.IgnoreNotFound(generator.MutateWebsite(ctx, c, obj, s.Labels))
		},
		Every: time.Minute,
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-mutator: %w", err)
	}

	return nil
}
