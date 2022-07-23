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
	"os"
	"strconv"

	"go.uber.org/zap/zapcore"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/config/v1alpha1"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/webhosting"
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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts.managerOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&webhosting.WebsiteReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("website-controller"),
		Config:   opts.controllerManagerConfig,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Website")
		os.Exit(1)
	}
	if err = (&webhosting.ThemeReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Theme")
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
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

type options struct {
	configFile string

	managerOptions          ctrl.Options
	controllerManagerConfig *configv1alpha1.ControllerManagerConfig
}

func (o *options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")
}

func (o *options) Complete() error {
	o.controllerManagerConfig = &configv1alpha1.ControllerManagerConfig{}

	var err error
	opts := ctrl.Options{Scheme: scheme}
	if o.configFile != "" {
		opts, err = opts.AndFrom(ctrl.ConfigFile().AtPath(o.configFile).OfKind(o.controllerManagerConfig))
		if err != nil {
			return err
		}
	}

	opts, err = applyOptionsOverrides(opts)
	if err != nil {
		return err
	}

	// apply some sensible defaults
	o.managerOptions = setOptionsDefaults(opts)
	return nil
}

func applyOptionsOverrides(opts ctrl.Options) (ctrl.Options, error) {
	// allow overriding leader election via env var for debugging purposes
	if leaderElectEnv, ok := os.LookupEnv("LEADER_ELECT"); ok {
		leaderElect, err := strconv.ParseBool(leaderElectEnv)
		if err != nil {
			return ctrl.Options{}, err
		}
		opts.LeaderElection = leaderElect
	}

	opts.Sharded = true
	// allow overriding shard ID
	opts.ShardID = os.Getenv("SHARD_ID")
	opts.CacheShardedFor = []client.Object{
		&webhostingv1alpha1.Website{},
	}

	return opts, nil
}

func setOptionsDefaults(opts ctrl.Options) ctrl.Options {
	if opts.HealthProbeBindAddress == "" { // "" disables the health server
		opts.HealthProbeBindAddress = ":8080"
	}

	// allows us to quickly handover leadership on restarts
	opts.LeaderElectionReleaseOnCancel = true

	return opts
}
