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
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	opts := newOptions()

	cmd := &cobra.Command{
		Use:   "shard",
		Short: "Run a dummy shard",
		Long: `The shard command runs a dummy shard that fulfills the requirements of a controller that supports sharding.
For this, it creates a shard Lease object and renews it periodically.
It also starts a controller for ConfigMaps that are assigned to the shard and handles the drain operation as expected.
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
	shardName       string
}

func newOptions() *options {
	return &options{
		zapOptions: &zap.Options{
			Development: true,
			TimeEncoder: zapcore.ISO8601TimeEncoder,
		},

		clusterRingName: "dummy",
		shardName:       "shard-" + rand.String(8),
	}
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.clusterRingName, "clusterring", o.clusterRingName, "Name of the ClusterRing the dummy shard belongs to.")
	fs.StringVar(&o.shardName, "shard", o.shardName, "Name of the dummy shard. Defaults to shard-<random-suffix>.")

	zapFlagSet := flag.NewFlagSet("zap", flag.ContinueOnError)
	o.zapOptions.BindFlags(zapFlagSet)
	fs.AddGoFlagSet(zapFlagSet)
}

func (o *options) validate() error {
	if o.shardName == "" {
		return fmt.Errorf("--shard must not be empty")
	}

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
	leaseClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("failed creating client for shard lease: %w", err)
	}

	shardLease := &LeaseLock{
		LeaseKey: client.ObjectKey{
			Namespace: metav1.NamespaceDefault,
			Name:      o.shardName,
		},
		Client: leaseClient,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: o.shardName,
		},
		Labels: map[string]string{
			shardingv1alpha1.LabelClusterRing: o.clusterRingName,
		},
	}

	log.Info("Setting up manager")
	shardLabelSelector := labels.SelectorFromSet(labels.Set{
		shardingv1alpha1.LabelShard(shardingv1alpha1.KindClusterRing, "", o.clusterRingName): o.shardName,
	})

	mgr, err := manager.New(restConfig, manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",

		GracefulShutdownTimeout: ptr.To(5 * time.Second),

		// Use manager's leader election mechanism for maintaining the shard lease.
		// With this, controllers will only run as long as manager holds the shard lease.
		// After graceful termination, the lease will be released.
		LeaderElection:                      true,
		LeaderElectionResourceLockInterface: shardLease,
		LeaderElectionReleaseOnCancel:       true,

		// Configure cache to watch only objects in the default namespace and that are assigned to this shard.
		Cache: cache.Options{
			DefaultNamespaces:    map[string]cache.Config{metav1.NamespaceDefault: {}},
			DefaultLabelSelector: shardLabelSelector,
		},
	})
	if err != nil {
		return fmt.Errorf("failed setting up manager: %w", err)
	}
	log.V(1).Info("Cache filtered with label selector", "selector", shardLabelSelector.String())

	log.Info("Setting up controller")
	if err := (&Reconciler{
		ClusterRingName: o.clusterRingName,
		ShardName:       o.shardName,
	}).AddToManager(mgr); err != nil {
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
	log.Info("Shutting down gracefully in 2 seconds, send another SIGINT or SIGTERM to shutdown non-gracefully")

	<-time.After(2 * time.Second)

	// signal manager to shut down, wait for it to terminate, and propagate the error it returned
	managerCancel()
	return <-managerDone
}
