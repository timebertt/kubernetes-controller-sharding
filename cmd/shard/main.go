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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	shardlease "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/lease"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	opts := newOptions()

	cmd := &cobra.Command{
		Use:   "shard",
		Short: "Run an example shard",
		Long: `The shard command runs an example shard that fulfills the requirements of a controller that supports sharding.
For this, it creates a shard Lease object and renews it periodically.
It also starts a controller for ConfigMaps that are assigned to the shard and handles the drain operation as expected.
See https://github.com/timebertt/kubernetes-controller-sharding/blob/main/docs/implement-sharding.md for more details.
This is basically a lightweight example controller which is useful for developing the sharding components without actually
running a full controller that complies with the sharding requirements.`,

		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}

			cmd.SilenceUsage = true

			return opts.run(cmd.Context())
		},
	}

	opts.AddFlags(cmd.Flags())

	if err := cmd.ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type options struct {
	zapOptions      *zap.Options
	clusterRingName string
	leaseNamespace  string
	shardName       string
}

func newOptions() *options {
	return &options{
		zapOptions: &zap.Options{
			Development: true,
			TimeEncoder: zapcore.ISO8601TimeEncoder,
		},

		clusterRingName: "example",
	}
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.clusterRingName, "clusterring", o.clusterRingName, "Name of the ClusterRing the shard belongs to.")
	fs.StringVar(&o.leaseNamespace, "lease-namespace", o.leaseNamespace, "Namespace to use for the shard lease. Defaults to the pod's namespace if running in-cluster.")
	fs.StringVar(&o.shardName, "shard", o.shardName, "Name of the shard. Defaults to the instance's hostname.")

	zapFlagSet := flag.NewFlagSet("zap", flag.ContinueOnError)
	o.zapOptions.BindFlags(zapFlagSet)
	fs.AddGoFlagSet(zapFlagSet)
}

func (o *options) validate() error {
	if o.clusterRingName == "" {
		return fmt.Errorf("--clusterring must not be empty")
	}

	return nil
}

func (o *options) run(ctx context.Context) error {
	log := zap.New(zap.UseFlagOptions(o.zapOptions))
	logf.SetLogger(log)
	klog.SetLogger(log)

	log.Info("Getting rest config")
	restConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed getting rest config: %w", err)
	}

	log.Info("Setting up shard lease")
	shardLease, err := shardlease.NewResourceLock(restConfig, nil, shardlease.Options{
		ClusterRingName: o.clusterRingName,
		LeaseNamespace:  o.leaseNamespace, // optional, can be empty
		ShardName:       o.shardName,      // optional, can be empty
	})
	if err != nil {
		return fmt.Errorf("failed creating shard lease: %w", err)
	}

	log.Info("Setting up manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",

		GracefulShutdownTimeout: ptr.To(5 * time.Second),

		// SHARD LEASE
		// Use manager's leader election mechanism for maintaining the shard lease.
		// With this, controllers will only run as long as manager holds the shard lease.
		// After graceful termination, the shard lease will be released.
		LeaderElection:                      true,
		LeaderElectionResourceLockInterface: shardLease,
		LeaderElectionReleaseOnCancel:       true,

		// FILTERED WATCH CACHE
		Cache: cache.Options{
			// This shard only acts on objects in the default namespace.
			DefaultNamespaces: map[string]cache.Config{metav1.NamespaceDefault: {}},
			// Configure cache to only watch objects that are assigned to this shard.
			// This shard only watches sharded objects, so we can configure the label selector on the cache's global level.
			// If your shard watches sharded objects as well as non-sharded objects, use cache.Options.ByObject to configure
			// the label selector on object level.
			DefaultLabelSelector: labels.SelectorFromSet(labels.Set{
				shardingv1alpha1.LabelShard(shardingv1alpha1.KindClusterRing, "", o.clusterRingName): shardLease.Identity(),
			}),
		},
	})
	if err != nil {
		return fmt.Errorf("failed setting up manager: %w", err)
	}

	log.Info("Setting up controller")
	if err := (&Reconciler{}).AddToManager(mgr, o.clusterRingName, shardLease.Identity()); err != nil {
		return fmt.Errorf("failed adding controller: %w", err)
	}

	log.Info("Starting manager")
	managerDone := make(chan error, 1)
	managerCtx, managerCancel := context.WithCancel(context.Background())

	go func() {
		managerDone <- mgr.Start(managerCtx)
	}()

	// Usually, SIGINT and SIGTERM trigger graceful termination immediately.
	// For development purposes, we allow simulating non-graceful termination by delaying cancellation of the manager.
	<-ctx.Done()
	log.Info("Shutting down gracefully in 2 seconds, send another SIGINT or SIGTERM to shut down non-gracefully")

	<-time.After(2 * time.Second)

	// signal manager to shut down, wait for it to terminate, and propagate the error it returned
	managerCancel()
	return <-managerDone
}
