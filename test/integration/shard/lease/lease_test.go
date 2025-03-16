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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	shardlease "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/lease"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test"
)

var _ = Describe("Shard lease", func() {
	var (
		mgrOptions manager.Options
		mgrCancel  context.CancelFunc

		leaseOptions shardlease.Options
		lease        *coordinationv1.Lease
	)

	BeforeEach(func() {
		mgrOptions = manager.Options{
			LeaderElection:             true,
			LeaderElectionResourceLock: "should-be-ignored", // -> should use provided lock instead
			LeaderElectionNamespace:    "should-be-ignored", // -> should use provided namespace in lock instead
			LeaderElectionID:           "should-be-ignored", // -> should be shard name instead

			LeaseDuration: ptr.To(time.Second),
			RenewDeadline: ptr.To(100 * time.Millisecond),
			RetryPeriod:   ptr.To(50 * time.Millisecond),
		}

		leaseOptions = shardlease.Options{
			ControllerRingName: "test-" + test.RandomSuffix(),
			LeaseNamespace:     testRunID,
			ShardName:          "test-" + test.RandomSuffix(),
		}

		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: leaseOptions.LeaseNamespace,
				Name:      leaseOptions.ShardName,
			},
		}
	}, OncePerOrdered)

	JustBeforeEach(func() {
		shardLease, err := shardlease.NewResourceLock(restConfig, leaseOptions)
		Expect(err).NotTo(HaveOccurred())
		mgrOptions.LeaderElectionResourceLockInterface = shardLease

		By("Setup manager")
		mgrOptions.Metrics.BindAddress = "0"
		mgr, err := manager.New(restConfig, mgrOptions)
		Expect(err).NotTo(HaveOccurred())

		By("Start manager")
		var mgrContext context.Context
		mgrContext, mgrCancel = context.WithCancel(context.Background())

		mgrDone := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrContext)).To(Succeed())
			close(mgrDone)
		}()

		DeferCleanup(func(ctx SpecContext) {
			By("Stop manager")
			mgrCancel()

			By("Wait for manager to stop")
			Eventually(ctx, mgrDone).Should(BeClosed())
		}, NodeTimeout(time.Minute))
	}, OncePerOrdered)

	Describe("should maintain the shard lease", Ordered, func() {
		It("should create the shard lease according to the config", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(And(
				HaveField("ObjectMeta.Labels", HaveKeyWithValue("alpha.sharding.timebertt.dev/controllerring", leaseOptions.ControllerRingName)),
				HaveField("Spec.HolderIdentity", HaveValue(Equal(leaseOptions.ShardName))),
				HaveField("Spec.LeaseDurationSeconds", HaveValue(BeEquivalentTo(1))),
				HaveField("Spec.AcquireTime", Not(BeNil())),
				HaveField("Spec.RenewTime", Not(BeNil())),
			))
		}, SpecTimeout(time.Minute))

		It("should renew the shard lease", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(And(
				HaveField("Spec.HolderIdentity", HaveValue(Equal(leaseOptions.ShardName))),
				HaveField("Spec.LeaseDurationSeconds", HaveValue(BeEquivalentTo(1))),
				HaveField("Spec.AcquireTime.Time", BeTemporally("~", lease.Spec.AcquireTime.Time)),
				HaveField("Spec.RenewTime.Time", BeTemporally(">", lease.Spec.RenewTime.Time)),
			))
		}, SpecTimeout(time.Minute))
	})

	Describe("should renew shard lease that was acquired by sharder", Ordered, func() {
		var sharderAcquireTime time.Time

		BeforeAll(func(ctx SpecContext) {
			sharderAcquireTime = time.Now()

			lease.Spec.HolderIdentity = ptr.To("sharder")
			lease.Spec.AcquireTime = ptr.To(metav1.NewMicroTime(sharderAcquireTime))
			lease.Spec.RenewTime = ptr.To(metav1.NewMicroTime(sharderAcquireTime))
			lease.Spec.LeaseDurationSeconds = ptr.To[int32](3)
			lease.Spec.LeaseTransitions = ptr.To[int32](1)

			Eventually(ctx, func() error {
				return testClient.Create(ctx, lease)
			}).Should(Succeed())
		}, NodeTimeout(time.Minute))

		It("should wait for lease to expire", func(ctx SpecContext) {
			Consistently(ctx, Object(lease)).WithTimeout(2 * time.Second).ShouldNot(
				HaveField("Spec.HolderIdentity", HaveValue(Equal(leaseOptions.ShardName))),
			)
		}, SpecTimeout(time.Minute))

		It("should renew the shard lease", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(And(
				HaveField("Spec.HolderIdentity", HaveValue(Equal(leaseOptions.ShardName))),
				HaveField("Spec.LeaseDurationSeconds", HaveValue(BeEquivalentTo(1))),
				HaveField("Spec.AcquireTime.Time", BeTemporally(">", sharderAcquireTime)),
				HaveField("Spec.RenewTime.Time", BeTemporally(">", sharderAcquireTime)),
				HaveField("Spec.LeaseTransitions", HaveValue(BeEquivalentTo(2))),
			))
		}, SpecTimeout(time.Minute))
	})

	When("LeaderElectionReleaseOnCancel is true", Ordered, func() {
		BeforeAll(func() {
			mgrOptions.LeaderElectionReleaseOnCancel = true
		})

		It("should create the shard lease", func(ctx SpecContext) {
			Eventually(ctx, Object(lease)).Should(
				HaveField("Spec.HolderIdentity", HaveValue(Equal(leaseOptions.ShardName))),
			)
		}, SpecTimeout(time.Minute))

		It("should release the shard lease when canceling the manager", func(ctx SpecContext) {
			mgrCancel()

			Eventually(ctx, Object(lease)).Should(
				HaveField("Spec.HolderIdentity", HaveValue(BeEmpty())),
			)
		}, SpecTimeout(time.Minute))
	})
})
