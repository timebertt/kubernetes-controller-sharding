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

package chaos

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/generator"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/scenario/base"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils"
)

const ScenarioName = "chaos"

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

// TODO
func (s *scenario) Description() string {
	return "Basic load test scenario (15m) that creates roughly 9k websites"
}

func (s *scenario) LongDescription() string {
	return `The ` + ScenarioName + ` scenario combines several operations typical for a lively operator environment:
- website creation: 10800 over 15m
- website deletion: 1800 over 15m
- website spec changes: 1/m per object, max 150/s
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
		Rate: rate.Limit(5),
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-generator: %w", err)
	}

	// trigger individual spec changes for website once per minute
	// => peaks at about 150 spec changes per second at the end of the experiment
	// (triggers roughly double the reconciliation rate in website controller because of deployment watches)
	if err := (&generator.ForEach[*webhostingv1alpha1.Website]{
		Name: "website-mutator",
		Do: func(ctx context.Context, c client.Client, obj *webhostingv1alpha1.Website) error {
			return client.IgnoreNotFound(generator.MutateWebsite(ctx, c, obj, s.Labels))
		},
		Every: 2 * time.Minute,
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding website-mutator: %w", err)
	}

	if err := (&generator.Every{
		Name: "shard-terminator",
		Do:   terminateRandomShard,
		Rate: rate.Every(3 * time.Minute),
	}).AddToManager(s.Manager); err != nil {
		return fmt.Errorf("error adding shard-terminator: %w", err)
	}

	return s.Wait(ctx, 15*time.Minute)
}

func terminateRandomShard(ctx context.Context, c client.Client) error {
	log := logf.Log.WithName("shard-terminator")

	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.InNamespace("webhosting-system"), client.MatchingLabels{"app.kubernetes.io/name": "webhosting-operator"}); err != nil {
		return err
	}

	if len(podList.Items) == 0 {
		log.Info("No shards found, skipping termination")
		return nil
	}

	pod := utils.PickRandom(podList.Items)
	if err := c.Delete(ctx, &pod); err != nil {
		return err
	}

	log.Info("Terminated shard", "pod", client.ObjectKeyFromObject(&pod))
	return nil
}
