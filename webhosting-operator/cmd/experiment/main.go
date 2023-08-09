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

package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"

	// Import all scenarios
	_ "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/scenario/all"
)

var (
	log    logr.Logger
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(webhostingv1alpha1.AddToScheme(scheme))
}

func main() {
	rand.Seed(time.Now().Unix())

	zapOpts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
	}

	var (
		mgr              manager.Manager
		selectedScenario experiment.Scenario
	)

	cmd := &cobra.Command{
		Use:  "experiment",
		Args: cobra.NoArgs,

		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
			log = ctrl.Log
			klog.SetLogger(ctrl.Log)

			restConfig := ctrl.GetConfigOrDie()
			restConfig.QPS = 1000
			restConfig.Burst = 1200

			var err error
			mgr, err = ctrl.NewManager(restConfig, ctrl.Options{
				Scheme: scheme,
				// disable leader election
				LeaderElection: false,
				// disable all endpoints
				HealthProbeBindAddress: "0",
				MetricsBindAddress:     "0",

				Controller: config.Controller{
					RecoverPanic: pointer.Bool(true),
				},
			})
			return err
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if err := selectedScenario.AddToManager(mgr); err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			go func() {
				select {
				case <-ctx.Done():
				case <-selectedScenario.Done():
					// stop the manager when scenario is done
					cancel()
				}
			}()

			log.Info("Starting manager")
			return mgr.Start(ctx)
		},
	}

	zapOpts.BindFlags(flag.CommandLine)
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)

	group := cobra.Group{
		ID:    "scenarios",
		Title: "Available Scenarios",
	}
	cmd.AddGroup(&group)

	for _, s := range experiment.GetAllScenarios() {
		scenario := s

		cmd.AddCommand(&cobra.Command{
			Use:     scenario.Name(),
			Short:   scenario.Description(),
			Long:    scenario.LongDescription(),
			Args:    cobra.NoArgs,
			GroupID: group.ID,
			Run: func(cmd *cobra.Command, args []string) {
				selectedScenario = scenario
			},
		})
	}

	if err := cmd.ExecuteContext(ctrl.SetupSignalHandler()); err != nil {
		fmt.Printf("Error running experiment: %v\n", err)
		os.Exit(1)
	}
}
