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
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils"

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

			// This is like importing the automaxprocs package for its init func (it will in turn call maxprocs.Set).
			// Here we pass a custom logger, so that the result of the library gets logged to the same logger we use for the
			// component itself.
			if _, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
				log.Info(fmt.Sprintf(s, i...))
			})); err != nil {
				log.Error(err, "Failed to set GOMAXPROCS")
			}

			restConfig := ctrl.GetConfigOrDie()
			restConfig.QPS = 1000
			restConfig.Burst = 1200

			var err error
			mgr, err = ctrl.NewManager(restConfig, ctrl.Options{
				Scheme:                 scheme,
				HealthProbeBindAddress: ":8081",
				MetricsBindAddress:     ":8080",
				// disable leader election
				LeaderElection: false,

				Controller: config.Controller{
					RecoverPanic: pointer.Bool(true),
				},
			})
			if err != nil {
				return err
			}

			if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
				return fmt.Errorf("unable to set up health check: %w", err)
			}
			if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
				return fmt.Errorf("unable to set up ready check: %w", err)
			}

			return nil
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
					if utils.RunningInCluster() {
						// but give monitoring a bit more time to scrape the final metrics values if running in a cluster
						<-time.After(time.Minute)
					}
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
