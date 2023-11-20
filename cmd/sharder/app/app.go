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

package app

import (
	"context"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/automaxprocs/maxprocs"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/timebertt/kubernetes-controller-sharding/pkg/controller"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/ring"
	healthzutils "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/healthz"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/webhook"
)

// Name is a const for the name of this component.
const Name = "sharder"

// NewCommand creates a new cobra.Command for running sharder.
func NewCommand() *cobra.Command {
	opts := newOptions()

	cmd := &cobra.Command{
		Use:   Name,
		Short: "Launch the " + Name,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			if err := opts.complete(); err != nil {
				return err
			}

			log := zap.New(zap.UseFlagOptions(opts.zapOptions))
			logf.SetLogger(log)
			klog.SetLogger(log)

			log.Info("Starting "+Name, "version", version.Get())
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				log.Info(fmt.Sprintf("FLAG: --%s=%s", flag.Name, flag.Value))
			})

			// don't output usage on further errors raised during execution
			cmd.SilenceUsage = true
			// further errors will be logged properly, don't duplicate
			cmd.SilenceErrors = true

			return run(cmd.Context(), log, opts)
		},
	}

	flags := cmd.Flags()
	verflag.AddFlags(flags)
	opts.addFlags(flags)

	return cmd
}

func run(ctx context.Context, log logr.Logger, opts *options) error {
	// This is like importing the automaxprocs package for its init func (it will in turn call maxprocs.Set).
	// Here we pass a custom logger, so that the result of the library gets logged to the same logger we use for the
	// component itself.
	if _, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		log.Info(fmt.Sprintf(s, i...))
	})); err != nil {
		log.Error(err, "Failed to set GOMAXPROCS")
	}

	// replace deprecated legacy go collector
	metrics.Registry.Unregister(collectors.NewGoCollector())
	metrics.Registry.MustRegister(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{Matcher: regexp.MustCompile("/.*")})),
	)

	log.Info("Setting up manager")
	mgr, err := manager.New(opts.restConfig, opts.managerOptions)
	if err != nil {
		return err
	}

	log.Info("Setting up health check endpoints")
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("cache-sync", healthzutils.CacheSync(mgr.GetCache())); err != nil {
		return err
	}

	ringCache := ring.NewCache()

	log.Info("Adding controllers to manager")
	if err := controller.AddToManager(ctx, mgr, ringCache, opts.config); err != nil {
		return fmt.Errorf("failed adding controllers to manager: %w", err)
	}

	log.Info("Adding webhooks to manager")
	if err := webhook.AddToManager(ctx, mgr, ringCache, opts.config); err != nil {
		return fmt.Errorf("failed adding webhooks to manager: %w", err)
	}

	log.Info("Starting manager")
	return mgr.Start(ctx)
}
