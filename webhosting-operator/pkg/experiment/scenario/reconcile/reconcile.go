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
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/scenario/base"
)

const ScenarioName = "reconcile"

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
	return "High frequency reconciliation load test scenario (15m) with a static number of websites (10k)"
}

func (s *scenario) LongDescription() string {
	return `The ` + ScenarioName + ` scenario generates 10k websites during preparation.
Then, it reconciles each website every 10s, i.e., it generates a reconciliation frequency of 1000/s.`
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

	s.Log.Info("Preparing websites")
	if err := generator.CreateWebsites(ctx, s.Client, 10000, generator.WithLabels(s.Labels)); err != nil {
		return err
	}

	return nil
}

func (s *scenario) Run(ctx context.Context) error {
	// trigger individual reconciliations for website every 10s
	if err := (&generator.ForEach[*webhostingv1alpha1.Website]{
		Name: "website-reconcile-trigger",
		Do: func(ctx context.Context, c client.Client, obj *webhostingv1alpha1.Website) error {
			return client.IgnoreNotFound(generator.ReconcileWebsite(ctx, c, obj))
		},
		Every:     10 * time.Second,
		RateLimit: rate.Limit(1000),
		Labels:    s.Labels,
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-reconcile-trigger: %w", err)
	}

	return nil
}
