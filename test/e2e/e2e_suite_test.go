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
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/komega"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Sharding E2E Test Suite")
}

const (
	ShortTimeout = 10 * time.Second
)

var (
	log logr.Logger

	testClient client.Client
)

var _ = BeforeSuite(func() {
	SetDefaultEventuallyPollingInterval(500 * time.Millisecond)
	SetDefaultEventuallyTimeout(time.Hour)
	SetDefaultConsistentlyPollingInterval(500 * time.Millisecond)
	SetDefaultConsistentlyDuration(5 * time.Second)

	log = zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

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

	komega.SetClient(testClient)
})
