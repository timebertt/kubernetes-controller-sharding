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
	"flag"
	"fmt"
	"os"
	goruntime "runtime"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/sharding"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/config/v1alpha1"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils/routes"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(webhostingv1alpha1.AddToScheme(scheme))
	utilruntime.Must(configv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	ctx := ctrl.SetupSignalHandler()
	opts := options{}
	opts.AddFlags(flag.CommandLine)

	zapOpts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	klog.SetLogger(ctrl.Log)

	if err := opts.Complete(); err != nil {
		setupLog.Error(err, "unable to load config")
		os.Exit(1)
	}

	// replace deprecated legacy go collector
	metrics.Registry.Unregister(collectors.NewGoCollector())
	metrics.Registry.MustRegister(collectors.NewGoCollector(collectors.WithGoCollections(collectors.GoRuntimeMetricsCollection)))

	mgr, err := ctrl.NewManager(opts.restConfig, opts.managerOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if *opts.config.Debugging.EnableProfiling {
		if err := (routes.Profiling{}).AddToManager(mgr); err != nil {
			setupLog.Error(err, "failed adding profiling handlers to manager")
			os.Exit(1)
		}
		if *opts.config.Debugging.EnableContentionProfiling {
			goruntime.SetBlockProfileRate(1)
		}
	}

	if err = (&webhosting.WebsiteReconciler{
		Config: opts.config,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Website")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

type options struct {
	configFile string

	restConfig     *rest.Config
	config         *configv1alpha1.WebhostingOperatorConfig
	managerOptions ctrl.Options
}

func (o *options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.configFile, "config", "",
		"Path to the file containing the operator's configuration (WebhostingOperatorConfig). "+
			"If not specified, the operator will use the default configuration values.")
}

func (o *options) Complete() error {
	o.config = &configv1alpha1.WebhostingOperatorConfig{}

	// load config file if specified
	if o.configFile != "" {
		configBytes, err := os.ReadFile(o.configFile)
		if err != nil {
			return fmt.Errorf("failed reading config file: %w", err)
		}

		if err := runtime.DecodeInto(serializer.NewCodecFactory(scheme).UniversalDecoder(), configBytes, o.config); err != nil {
			return fmt.Errorf("failed decoding config file: %w", err)
		}
	} else {
		scheme.Default(o.config)
	}

	// load rest config
	var err error
	o.restConfig, err = ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed loading kubeconfig: %w", err)
	}

	// bring everything together
	o.managerOptions = ctrl.Options{
		Scheme: scheme,
		// allows us to quickly handover leadership on restarts
		LeaderElectionReleaseOnCancel: true,
		Cache: cache.Options{
			DefaultTransform: dropUnwantedMetadata,
		},
		Controller: config.Controller{
			RecoverPanic: pointer.Bool(true),
		},
	}

	o.applyConfigToRESTConfig()
	o.applyConfigToOptions()
	if err := o.applyOptionsOverrides(); err != nil {
		return err
	}

	return nil
}

func (o *options) applyConfigToRESTConfig() {
	if clientConnection := o.config.ClientConnection; clientConnection != nil {
		if clientConnection.QPS > 0 {
			o.restConfig.QPS = clientConnection.QPS
		}
		if clientConnection.Burst > 0 {
			o.restConfig.Burst = int(clientConnection.Burst)
		}
	}
}

func (o *options) applyConfigToOptions() {
	if leaderElection := o.config.LeaderElection; leaderElection != nil {
		o.managerOptions.LeaderElection = *leaderElection.LeaderElect
		o.managerOptions.LeaderElectionResourceLock = leaderElection.ResourceLock
		o.managerOptions.LeaderElectionID = leaderElection.ResourceName
		o.managerOptions.LeaderElectionNamespace = leaderElection.ResourceNamespace
		o.managerOptions.LeaseDuration = pointer.Duration(leaderElection.LeaseDuration.Duration)
		o.managerOptions.RenewDeadline = pointer.Duration(leaderElection.RenewDeadline.Duration)
		o.managerOptions.RetryPeriod = pointer.Duration(leaderElection.RetryPeriod.Duration)
	}

	o.managerOptions.HealthProbeBindAddress = o.config.Health.BindAddress
	o.managerOptions.MetricsBindAddress = o.config.Metrics.BindAddress
	o.managerOptions.GracefulShutdownTimeout = pointer.Duration(o.config.GracefulShutdownTimeout.Duration)
}

func (o *options) applyOptionsOverrides() error {
	var err error

	// allow overriding leader election via env var for debugging purposes
	if leaderElectEnv, ok := os.LookupEnv("LEADER_ELECT"); ok {
		o.managerOptions.LeaderElection, err = strconv.ParseBool(leaderElectEnv)
		if err != nil {
			return fmt.Errorf("error parsing LEADER_ELECT env var: %w", err)
		}
	}

	o.managerOptions.Sharded = true
	// allow disabling sharding via env var for evaluation
	if shardingEnv, ok := os.LookupEnv("SHARDING_ENABLED"); ok {
		o.managerOptions.Sharded, err = strconv.ParseBool(shardingEnv)
		if err != nil {
			return fmt.Errorf("error parsing SHARDING_ENABLED env var: %w", err)
		}
	}

	// allow overriding shard ID via env var
	o.managerOptions.ShardID = os.Getenv("SHARD_ID")
	o.managerOptions.ShardMode = sharding.Mode(os.Getenv("SHARD_MODE"))

	return nil
}

func dropUnwantedMetadata(i interface{}) (interface{}, error) {
	obj, ok := i.(client.Object)
	if !ok {
		return i, nil
	}

	obj.SetManagedFields(nil)
	annotations := obj.GetAnnotations()
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	obj.SetAnnotations(annotations)

	return obj, nil
}
