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

package e2e

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhosting Operator E2E Test Suite")
}

const (
	testID = "e2e-webhosting-operator"

	ShortTimeout  = 10 * time.Second
	MediumTimeout = time.Minute
)

var (
	log logr.Logger

	testClient client.Client

	testRunID     string
	testRunLabels map[string]string
)

var _ = BeforeSuite(func() {
	log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)).WithName(testID)

	restConfig, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	// gotta go fast during tests
	restConfig.QPS = 100
	restConfig.Burst = 150

	scheme := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		shardingv1alpha1.AddToScheme,
		webhostingv1alpha1.AddToScheme,
	)
	Expect(schemeBuilder.AddToScheme(scheme)).To(Succeed())

	testClient, err = client.New(restConfig, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	clientContext, clientCancel := context.WithCancel(context.Background())
	komega.SetClient(testClient)
	komega.SetContext(clientContext)
	DeferCleanup(clientCancel)

	testRunID = testID + "-" + test.RandomSuffix()
	testRunLabels = map[string]string{
		testID: testRunID,
	}
	log = log.WithValues("testRun", testRunID)
})

var (
	controllerRing *shardingv1alpha1.ControllerRing
	namespace      *corev1.Namespace

	controllerDeployment *appsv1.Deployment
)

var _ = BeforeEach(func(ctx SpecContext) {
	By("Set up test Namespace")
	namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		GenerateName: testRunID + "-",
		Labels:       maps.Clone(testRunLabels),
	}}
	namespace.Labels[webhostingv1alpha1.LabelKeyProject] = webhostingv1alpha1.LabelValueProject

	// We create a dedicated test namespace and clean it up for every test case to ensure a clean test environment.
	Expect(testClient.Create(ctx, namespace)).To(Succeed())
	log.Info("Created test Namespace", "namespace", namespace.Name)

	DeferCleanup(func(ctx SpecContext) {
		By("Delete all Websites in test Namespace")
		Expect(testClient.DeleteAllOf(ctx, &webhostingv1alpha1.Website{}, client.InNamespace(namespace.Name))).To(Succeed())

		By("Delete test Namespace")
		Eventually(ctx, func() error {
			return testClient.Delete(ctx, namespace)
		}).Should(Or(Succeed(), BeNotFoundError()))
	}, NodeTimeout(MediumTimeout))

	By("Set up test ControllerRing")
	controllerRing = &shardingv1alpha1.ControllerRing{ObjectMeta: metav1.ObjectMeta{Name: webhostingv1alpha1.WebhostingOperatorName}}

	By("Scaling checksum-controller")
	controllerDeployment = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: webhostingv1alpha1.WebhostingOperatorName, Namespace: webhostingv1alpha1.NamespaceSystem}}
	scaleController(ctx, 3)

	DeferCleanup(func(ctx SpecContext) {
		By("Scaling checksum-controller")
		scaleController(ctx, 3)
	}, NodeTimeout(ShortTimeout))

	By("Set up test Theme")
	theme = &webhostingv1alpha1.Theme{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace.Name,
			Labels: maps.Clone(testRunLabels),
		},
		Spec: webhostingv1alpha1.ThemeSpec{
			Color:      "cyan",
			FontFamily: "arial",
		},
	}
	Expect(controllerutil.SetOwnerReference(namespace, theme, testClient.Scheme(), controllerutil.WithBlockOwnerDeletion(true))).To(Succeed())
	Expect(testClient.Create(ctx, theme)).To(Succeed())
}, NodeTimeout(MediumTimeout), OncePerOrdered)
