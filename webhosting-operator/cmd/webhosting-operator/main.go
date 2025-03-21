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
	"net/http"
	"os"
	goruntime "runtime"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	shardlease "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/lease"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/routes"
	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/config/v1alpha1"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting"
	webhostingmetrics "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/metrics"
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

	// This is like importing the automaxprocs package for its init func (it will in turn call maxprocs.Set).
	// Here we pass a custom logger, so that the result of the library gets logged to the same logger we use for the
	// component itself.
	if _, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		setupLog.Info(fmt.Sprintf(s, i...))
	})); err != nil {
		setupLog.Error(err, "Failed to set GOMAXPROCS")
	}

	// replace deprecated legacy go collector
	metrics.Registry.Unregister(collectors.NewGoCollector())
	metrics.Registry.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll)))

	mgr, err := ctrl.NewManager(opts.restConfig, opts.managerOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&webhosting.WebsiteReconciler{
		Config: opts.config,
	}).SetupWithManager(mgr, opts.enableSharding, opts.controllerRingName, opts.shardName); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Website")
		os.Exit(1)
	}

	if err = webhostingmetrics.AddToManager(mgr); err != nil {
		setupLog.Error(err, "unable to add metrics exporters")
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

	restConfig         *rest.Config
	config             *configv1alpha1.WebhostingOperatorConfig
	managerOptions     ctrl.Options
	enableSharding     bool
	controllerRingName string
	shardName          string
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
		o.managerOptions.LeaseDuration = ptr.To(leaderElection.LeaseDuration.Duration)
		o.managerOptions.RenewDeadline = ptr.To(leaderElection.RenewDeadline.Duration)
		o.managerOptions.RetryPeriod = ptr.To(leaderElection.RetryPeriod.Duration)
	}

	o.managerOptions.HealthProbeBindAddress = o.config.Health.BindAddress

	if o.config.Metrics.BindAddress != "0" {
		var extraHandlers map[string]http.Handler
		if *o.config.Debugging.EnableProfiling {
			extraHandlers = routes.ProfilingHandlers
			if *o.config.Debugging.EnableContentionProfiling {
				goruntime.SetBlockProfileRate(1)
			}
		}

		o.managerOptions.Metrics = metricsserver.Options{
			SecureServing:  true,
			BindAddress:    o.config.Metrics.BindAddress,
			FilterProvider: filters.WithAuthenticationAndAuthorization,
			ExtraHandlers:  extraHandlers,
		}
	}

	o.managerOptions.GracefulShutdownTimeout = ptr.To(o.config.GracefulShutdownTimeout.Duration)
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

	// allow enabling/disabling sharding via env var for evaluation
	if shardingEnv, ok := os.LookupEnv("ENABLE_SHARDING"); ok {
		o.enableSharding, err = strconv.ParseBool(shardingEnv)
		if err != nil {
			return fmt.Errorf("error parsing ENABLE_SHARDING env var: %w", err)
		}
	}

	if o.enableSharding {
		if !o.managerOptions.LeaderElection {
			return fmt.Errorf("sharding cannot be enabled if leader election is disabled")
		}

		// SHARD LEASE
		o.controllerRingName = webhostingv1alpha1.WebhostingOperatorName
		shardLease, err := shardlease.NewResourceLock(o.restConfig, shardlease.Options{
			ControllerRingName: o.controllerRingName,
		})
		if err != nil {
			return fmt.Errorf("failed creating shard lease: %w", err)
		}
		o.shardName = shardLease.Identity()

		// Use manager's leader election mechanism for maintaining the shard lease.
		// With this, controllers will only run as long as manager holds the shard lease.
		// After graceful termination, the shard lease will be released.
		o.managerOptions.LeaderElectionResourceLockInterface = shardLease

		// FILTERED WATCH CACHE
		// Configure cache to only watch objects that are assigned to this shard.
		shardLabelSelector := labels.SelectorFromSet(labels.Set{
			shardingv1alpha1.LabelShard(o.controllerRingName): o.shardName,
		})

		// This operator watches sharded objects (Websites, etc.) as well as non-sharded objects (Themes),
		// use cache.Options.ByObject to configure the label selector on object level.
		o.managerOptions.Cache.ByObject = map[client.Object]cache.ByObject{
			&webhostingv1alpha1.Website{}: {Label: shardLabelSelector},
			&appsv1.Deployment{}:          {Label: shardLabelSelector},
			&corev1.ConfigMap{}:           {Label: shardLabelSelector},
			&corev1.Service{}:             {Label: shardLabelSelector},
			&networkingv1.Ingress{}:       {Label: shardLabelSelector},
		}
	}

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
