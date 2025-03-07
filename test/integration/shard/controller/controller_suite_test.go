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

package controller_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
	. "github.com/timebertt/kubernetes-controller-sharding/test/integration/shard/controller"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shard Controller Integration Test Suite")
}

const testID = "shard-controller-test"

var (
	log logr.Logger

	testClient client.Client

	controllerRingName, shardName string
	shardLabel, drainLabel        string

	testRunID string
)

var _ = BeforeSuite(func(ctx SpecContext) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	log = logf.Log.WithName(testID)

	By("Start test environment")
	testEnv := &envtest.Environment{}

	restConfig, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	DeferCleanup(func() {
		By("Stop test environment")
		Expect(testEnv.Stop()).To(Succeed())
	})

	By("Create test clients")
	testClient, err = client.New(restConfig, client.Options{})
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

	controllerRingName = testRunID
	shardName = testRunID
	shardLabel = "shard.alpha.sharding.timebertt.dev/" + controllerRingName
	drainLabel = "drain.alpha.sharding.timebertt.dev/" + controllerRingName

	DeferCleanup(func(ctx SpecContext) {
		By("Delete test Namespace")
		Expect(testClient.Delete(ctx, testNamespace)).To(Or(Succeed(), BeNotFoundError()))
	}, NodeTimeout(time.Minute))

	By("Setup manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{testNamespace.Name: {}},
		},
	})
	Expect(err).NotTo(HaveOccurred())

	By("Register controller")
	Expect((&Reconciler{}).AddToManager(mgr, controllerRingName, shardName)).To(Succeed())

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
