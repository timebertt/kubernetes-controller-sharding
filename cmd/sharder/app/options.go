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
	"flag"
	"fmt"
	"net/http"
	"os"
	goruntime "runtime"
	"strconv"

	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/selection"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/routes"
)

var scheme = runtime.NewScheme()

func init() {
	schemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		shardingv1alpha1.AddToScheme,
		configv1alpha1.AddToScheme,
	)
	utilruntime.Must(schemeBuilder.AddToScheme(scheme))
}

type options struct {
	configFile string
	zapOptions *zap.Options

	config         *configv1alpha1.SharderConfig
	restConfig     *rest.Config
	managerOptions manager.Options
}

func newOptions() *options {
	return &options{
		zapOptions: &zap.Options{
			Development: true,
			TimeEncoder: zapcore.ISO8601TimeEncoder,
		},
	}
}

func (o *options) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.configFile, "config", o.configFile, "Path to configuration file.")

	zapFlagSet := flag.NewFlagSet("zap", flag.ContinueOnError)
	o.zapOptions.BindFlags(zapFlagSet)
	fs.AddGoFlagSet(zapFlagSet)
}

func (o *options) complete() error {
	o.config = &configv1alpha1.SharderConfig{}

	// load config file if specified
	if o.configFile != "" {
		data, err := os.ReadFile(o.configFile)
		if err != nil {
			return fmt.Errorf("error reading config file: %w", err)
		}

		if err = runtime.DecodeInto(serializer.NewCodecFactory(scheme).UniversalDecoder(), data, o.config); err != nil {
			return fmt.Errorf("error decoding config: %w", err)
		}
	} else {
		scheme.Default(o.config)
	}

	// load rest config
	var err error
	o.restConfig, err = config.GetConfig()
	if err != nil {
		return fmt.Errorf("error loading kubeconfig: %w", err)
	}
	o.applyConfigToRESTConfig()

	// bring everything together
	o.managerOptions = manager.Options{
		Scheme: scheme,
		// allows us to quickly handover leadership on restarts
		LeaderElectionReleaseOnCancel: true,
		Controller: controllerconfig.Controller{
			RecoverPanic: ptr.To(true),
		},
	}
	o.applyConfigToManagerOptions()
	o.applyCacheOptions()

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

func (o *options) applyConfigToManagerOptions() {
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

	webhookOptions := webhook.Options{}
	if serverConfig := o.config.Webhook.Server; serverConfig != nil {
		webhookOptions.CertDir = ptr.Deref(serverConfig.CertDir, "")
		webhookOptions.CertName = ptr.Deref(serverConfig.CertName, "")
		webhookOptions.KeyName = ptr.Deref(serverConfig.KeyName, "")
	}
	o.managerOptions.WebhookServer = webhook.NewServer(webhookOptions)

	o.managerOptions.GracefulShutdownTimeout = ptr.To(o.config.GracefulShutdownTimeout.Duration)
}

func (o *options) applyCacheOptions() {
	// filter lease cache for shard leases to avoid watching all leases in cluster
	leaseSelector := labels.NewSelector()
	{
		ringRequirement, err := labels.NewRequirement(shardingv1alpha1.LabelClusterRing, selection.Exists, nil)
		utilruntime.Must(err)
		leaseSelector.Add(*ringRequirement)
	}

	o.managerOptions.Cache = cache.Options{
		DefaultTransform: dropUnwantedMetadata,

		ByObject: map[client.Object]cache.ByObject{
			&coordinationv1.Lease{}: {
				Label: leaseSelector,
			},
		},
	}
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
