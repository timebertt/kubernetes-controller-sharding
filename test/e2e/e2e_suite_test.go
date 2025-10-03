/*
Copyright 2024 Tim Ebert.

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
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Sharding E2E Test Suite")
}

const (
	testID = "e2e-controller-sharding"

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

	scheme := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(
		clientgoscheme.AddToScheme,
		shardingv1alpha1.AddToScheme,
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

	controller *appsv1.StatefulSet
)

var _ = BeforeEach(func(ctx SpecContext) {
	By("Set up test Namespace")
	namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		GenerateName: testRunID + "-",
		Labels:       maps.Clone(testRunLabels),
	}}

	// We create a dedicated test namespace and clean it up for every test case to ensure a clean test environment.
	Expect(testClient.Create(ctx, namespace)).To(Succeed())
	log.Info("Created test Namespace", "namespace", namespace.Name)

	DeferCleanup(func(ctx SpecContext) {
		By("Delete test Namespace")
		Eventually(ctx, func() error {
			return testClient.Delete(ctx, namespace)
		}).Should(Or(Succeed(), BeNotFoundError()))
	}, NodeTimeout(MediumTimeout))

	By("Set up test ControllerRing")
	// Deploy a dedicated ControllerRing instance for this test case
	defaultControllerRing := &shardingv1alpha1.ControllerRing{ObjectMeta: metav1.ObjectMeta{Name: checksumControllerName}}
	Expect(komega.Get(defaultControllerRing)()).To(Succeed())

	controllerRing = defaultControllerRing.DeepCopy()
	controllerRing.Name = namespace.Name
	controllerRing.ResourceVersion = ""
	maps.Copy(controllerRing.Labels, testRunLabels)
	controllerRing.Spec.NamespaceSelector.MatchLabels[corev1.LabelMetadataName] = namespace.Name
	Expect(testClient.Create(ctx, controllerRing)).To(Succeed())

	DeferCleanup(func(ctx SpecContext) {
		By("Delete test ControllerRing")
		Eventually(ctx, func() error {
			return testClient.Delete(ctx, controllerRing)
		}).Should(Or(Succeed(), BeNotFoundError()))
	}, NodeTimeout(ShortTimeout))

	By("Set up test controller")
	// TODO: test with both Deployment and StatefulSet
	controller = &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace.Name, Name: checksumControllerName}}

	// Deploy a dedicated controller instance to this test case's namespace.
	// Copy all relevant objects from the default namespace.
	for _, objList := range []client.ObjectList{
		&appsv1.StatefulSetList{},
		&corev1.ServiceAccountList{},
		&rbacv1.RoleList{},
		&rbacv1.RoleBindingList{},
	} {
		Expect(testClient.List(ctx, objList, client.InNamespace(metav1.NamespaceDefault), client.MatchingLabels{"app.kubernetes.io/component": checksumControllerName})).
			Should(Succeed(), "should list %T in default namespace", objList)

		Expect(meta.EachListItem(objList, func(object runtime.Object) error {
			obj := object.DeepCopyObject().(client.Object)
			obj.SetNamespace(namespace.Name)
			obj.SetResourceVersion("")

			switch o := obj.(type) {
			case *appsv1.StatefulSet:
				o.Spec.Replicas = ptr.To[int32](3)
				o.Spec.Template.Spec.Containers[0].Args = append(o.Spec.Template.Spec.Containers[0].Args,
					"--controllerring="+controllerRing.Name,
					"--namespace="+namespace.Name,
				)
			case *rbacv1.RoleBinding:
				o.Subjects[0].Namespace = namespace.Name
			}

			if err := testClient.Create(ctx, obj); err != nil {
				return fmt.Errorf("error copying object %T %q to %s namespace: %w", obj, client.ObjectKeyFromObject(obj), namespace.Name, err)
			}
			return nil
		})).To(Succeed(), "should copy %T", objList)
	}
}, NodeTimeout(MediumTimeout), OncePerOrdered)
