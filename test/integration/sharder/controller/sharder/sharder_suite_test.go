/*
Copyright 2025 Tim Ebert.

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

package sharder_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	testclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/controller/sharder"
	utilclient "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/client"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

func TestSharder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sharder Controller Integration Test Suite")
}

const testID = "sharder-controller-test"

var (
	log logr.Logger

	testClient client.Client
	mgrClient  client.Client

	clock *testclock.FakePassiveClock

	testRunID     string
	testRunLabels map[string]string

	controllerRing *shardingv1alpha1.ControllerRing
)

var _ = BeforeSuite(func(ctx SpecContext) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName(testID)

	By("Start test environment")
	testEnv := &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{test.PathShardingCRDs()},
		},
		ErrorIfCRDPathMissing: true,
	}

	restConfig, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("Stop test environment")
		Expect(testEnv.Stop()).To(Succeed())
	})

	By("Create test clients")
	testClient, err = client.New(restConfig, client.Options{Scheme: utilclient.SharderScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(testClient).NotTo(BeNil())

	clientContext, clientCancel := context.WithCancel(context.Background())
	komega.SetClient(testClient)
	komega.SetContext(clientContext)
	DeferCleanup(clientCancel)

	By("Create test Namespace")
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: testID + "-",
		},
	}
	Expect(testClient.Create(ctx, testNamespace)).To(Succeed())
	log.Info("Created Namespace for test", "namespaceName", testNamespace.Name)
	testRunID = testNamespace.Name
	log = log.WithValues("testRunID", testRunID)
	testRunLabels = map[string]string{testID: testRunID}

	DeferCleanup(func(ctx SpecContext) {
		By("Delete test Namespace")
		Expect(testClient.Delete(ctx, testNamespace)).To(Or(Succeed(), BeNotFoundError()))
	}, NodeTimeout(time.Minute))

	By("Create test ControllerRing")
	controllerRing = &shardingv1alpha1.ControllerRing{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testRunID,
			Labels: maps.Clone(testRunLabels),
		},
		Spec: shardingv1alpha1.ControllerRingSpec{
			Resources: []shardingv1alpha1.RingResource{{
				GroupResource:       metav1.GroupResource{Group: "", Resource: "secrets"},
				ControlledResources: []metav1.GroupResource{{Group: "", Resource: "configmaps"}},
			}},
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      corev1.LabelMetadataName,
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{testRunID},
				}},
			},
		},
	}
	Expect(testClient.Create(ctx, controllerRing)).To(Succeed())
	log.Info("Created ControllerRing for test", "controllerRingName", controllerRing.Name)

	DeferCleanup(func(ctx SpecContext) {
		By("Delete test ControllerRing")
		Expect(testClient.Delete(ctx, controllerRing)).To(Or(Succeed(), BeNotFoundError()))
	}, NodeTimeout(time.Minute))

	By("Setup manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Scheme:  utilclient.SharderScheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				testNamespace.Name:      {},
				metav1.NamespaceDefault: {},
			},
			ByObject: map[client.Object]cache.ByObject{
				&shardingv1alpha1.ControllerRing{}: {
					Label: labels.SelectorFromSet(testRunLabels),
				},
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	mgrClient = mgr.GetClient()

	By("Register controller")
	config := &configv1alpha1.SharderConfig{}
	mgr.GetScheme().Default(config)

	clock = testclock.NewFakePassiveClock(time.Now())

	Expect((&sharder.Reconciler{
		Clock:  clock,
		Config: config,
	}).AddToManager(mgr)).To(Succeed())

	By("Start manager")
	mgrContext, mgrCancel := context.WithCancel(context.Background())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrContext)).To(Succeed())
	}()

	DeferCleanup(func() {
		By("Stop manager")
		mgrCancel()
	})
}, NodeTimeout(time.Minute))
