/*
Copyright 2025 Tim Ebert.

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

package rolling_update

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/scenario/base"
)

const ScenarioName = "rolling-update"

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
	return "Create 9k websites in 15 minutes while rolling the operator"
}

func (s *scenario) LongDescription() string {
	return `The ` + ScenarioName + ` scenario combines several operations typical for a lively operator environment with rolling updates:
- website creation: 10800 over 15m
- website deletion: 1800 over 15m
- website spec changes: 1/m per object, max 150/s
- rolling update of webhosting-operator: 1 every 5m
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
	// website-deleter:   deletes about  1800 websites over  15 minutes
	// => in total, there will be about  9000 websites after 15 minutes
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
	// => peaks at about 150 spec changes per second at the end of the experiment
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

	// trigger a rolling update of the webhosting-operator every 5 minutes
	if err := (&generator.Every{
		Name: "rolling-updater",
		Do:   triggerRollingUpdate,
		Rate: rate.Every(5 * time.Minute),
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding rolling-updater: %w", err)
	}

	return s.Wait(ctx, 15*time.Minute)
}

func triggerRollingUpdate(ctx context.Context, c client.Client) error {
	key := client.ObjectKey{Namespace: webhostingv1alpha1.NamespaceSystem, Name: webhostingv1alpha1.WebhostingOperatorName}

	var object client.Object = &appsv1.Deployment{}
	if err := c.Get(ctx, key, object); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		object = &appsv1.StatefulSet{}
		if err := c.Get(ctx, key, object); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}

			panic("neither Deployment nor StatefulSet found for webhosting-operator, aborting experiment")
		}
	}

	patch := client.RawPatch(types.MergePatchType, []byte(`{"spec":{"template":{"metadata":{"annotations":{"rolling-update":"`+time.Now().UTC().Format(time.RFC3339)+`"}}}}}`))
	if err := c.Patch(ctx, object, patch); err != nil {
		return err
	}

	logf.FromContext(ctx).Info("Triggered rolling update")
	return nil
}
