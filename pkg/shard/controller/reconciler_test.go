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
	"fmt"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/shard/controller"
)

var _ = Describe("#Reconciler", func() {
	var (
		ctx context.Context

		logBuffer *Buffer

		controllerRingName string
		shardName          string

		fakeClient client.Client

		r *Reconciler

		obj *corev1.Pod
		req reconcile.Request
	)

	BeforeEach(func() {
		logBuffer = NewBuffer()
		ctx = logf.IntoContext(context.Background(), zap.New(zap.UseDevMode(true), zap.WriteTo(io.MultiWriter(logBuffer, GinkgoWriter))))

		controllerRingName = "operator"
		shardName = "operator-0"

		fakeClient = fakeclient.NewFakeClient()

		r = &Reconciler{
			Object:             &corev1.Pod{},
			Client:             fakeClient,
			ControllerRingName: controllerRingName,
			ShardName:          shardName,
		}

		obj = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
		}
		req = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
	})

	JustBeforeEach(func() {
		if obj != nil {
			Expect(fakeClient.Create(ctx, obj)).To(Succeed())
		}
	})

	When("the object does not exist", func() {
		BeforeEach(func() {
			obj = nil
		})

		It("should ignore the request", func() {
			Expect(r.Reconcile(ctx, req)).To(BeZero())
			Eventually(logBuffer).Should(Say("Object is gone"))
		})
	})

	When("the object is assigned to another shard", func() {
		BeforeEach(func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "shard.alpha.sharding.timebertt.dev/"+controllerRingName, "other")
		})

		It("should ignore the request", func() {
			Expect(r.Reconcile(ctx, req)).To(BeZero())
			Eventually(logBuffer).Should(Say("Ignoring object as it is not assigned to this shard"))
		})
	})

	When("the object is assigned to this shard", func() {
		BeforeEach(func() {
			metav1.SetMetaDataLabel(&obj.ObjectMeta, "shard.alpha.sharding.timebertt.dev/"+controllerRingName, shardName)
		})

		When("the object is drained", func() {
			BeforeEach(func() {
				metav1.SetMetaDataLabel(&obj.ObjectMeta, "drain.alpha.sharding.timebertt.dev/"+controllerRingName, "true")
			})

			It("should remove the shard and drain label", func() {
				Expect(r.Reconcile(ctx, req)).To(BeZero())
				Eventually(logBuffer).Should(Say("Draining object"))

				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
				Expect(obj.GetLabels()).Should(BeEmpty())
			})
		})

		When("the object is not drained", func() {
			var (
				result reconcile.Result
				err    error
				called int
			)

			BeforeEach(func() {
				result, err = reconcile.Result{}, nil
				called = 0

				r.Do = reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
					called++
					return result, err
				})
			})

			It("should call the reconciler and return its result", func() {
				Expect(r.Reconcile(ctx, req)).To(BeZero())
				Expect(called).To(Equal(1))

				result = reconcile.Result{Requeue: true}
				Expect(r.Reconcile(ctx, req)).To(Equal(reconcile.Result{Requeue: true}))
				Expect(called).To(Equal(2))

				result = reconcile.Result{}
				err = fmt.Errorf("foo")
				Expect(r.Reconcile(ctx, req)).Error().To(MatchError("foo"))
				Expect(called).To(Equal(3))
			})
		})
	})
})
