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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/controller"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("#Builder", func() {
	var (
		mgr fakeManager
		b   *Builder

		obj                *corev1.Pod
		controllerRingName string
		shardName          string
		client             client.Client
		r                  reconcile.Reconciler
	)

	BeforeEach(func() {
		obj = &corev1.Pod{}
		controllerRingName = "operator"
		shardName = "operator-0"
		client = fakeclient.NewFakeClient()
		r = reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
			return reconcile.Result{}, nil
		})
	})

	JustBeforeEach(func() {
		mgr = fakeManager{Client: client}
		b = NewShardedReconciler(mgr).
			For(obj).
			InControllerRing(controllerRingName).
			WithShardName(shardName)
	})

	Describe("#For", func() {
		It("should complain about calling For twice", func() {
			Expect(b.For(obj).Build(r)).Error().To(MatchError("must not call For() more than once"))
		})

		It("should complain about not calling For", func() {
			b = NewShardedReconciler(mgr).
				InControllerRing(controllerRingName).
				WithShardName(shardName)
			Expect(b.Build(r)).Error().To(MatchError("missing object kind, must call to For()"))
		})
	})

	Describe("#WithClient", func() {
		It("should use the custom client instead of the manager's client", func() {
			customClient := fakeclient.NewFakeClient()
			Expect(b.WithClient(customClient).Build(r)).To(
				HaveField("Client", BeIdenticalTo(customClient)),
			)
		})

		It("should complain about missing client", func() {
			Expect(b.WithClient(nil).Build(r)).Error().To(MatchError("missing client"))
		})
	})

	Describe("#InControllerRing", func() {
		It("should complain about missing ControllerRing name", func() {
			Expect(b.InControllerRing("").Build(r)).Error().To(MatchError("missing ControllerRing name"))
		})
	})

	Describe("#WithShardName", func() {
		It("should complain about missing shard name", func() {
			Expect(b.WithShardName("").Build(r)).Error().To(MatchError("missing shard name"))
		})
	})

	Describe("#Build", func() {
		It("should complain about nil reconciler", func() {
			Expect(b.Build(nil)).Error().To(MatchError("must provide a non-nil Reconciler"))
		})

		It("should correctly set up the Reconciler", func() {
			shardReconciler, err := b.Build(r)
			Expect(err).NotTo(HaveOccurred())

			reconciler, ok := shardReconciler.(*Reconciler)
			Expect(ok).To(BeTrue())

			Expect(reconciler.Object).To(BeAssignableToTypeOf(obj))
			Expect(reconciler.Client).To(BeIdenticalTo(client))
			Expect(reconciler.ControllerRingName).To(Equal(controllerRingName))
			Expect(reconciler.ShardName).To(Equal(shardName))
			Expect(reconciler.Do).To(BeFunc(r))
		})
	})

	Describe("#MustBuild", func() {
		It("should panic for nil reconciler", func() {
			Expect(func() {
				b.MustBuild(nil)
			}).To(Panic())
		})

		It("should correctly set up the Reconciler", func() {
			var shardReconciler reconcile.Reconciler
			Expect(func() {
				shardReconciler = b.MustBuild(r)
			}).NotTo(Panic())

			reconciler, ok := shardReconciler.(*Reconciler)
			Expect(ok).To(BeTrue())

			Expect(reconciler.Object).To(BeAssignableToTypeOf(obj))
			Expect(reconciler.Client).To(BeIdenticalTo(client))
			Expect(reconciler.ControllerRingName).To(Equal(controllerRingName))
			Expect(reconciler.ShardName).To(Equal(shardName))
			Expect(reconciler.Do).To(BeFunc(r))
		})
	})
})

type fakeManager struct {
	manager.Manager
	Client client.Client
}

func (f fakeManager) GetClient() client.Client {
	return f.Client
}
