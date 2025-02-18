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

package lease

import (
	"context"
	"os"
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("LeaseLock", func() {
	const (
		namespace          = "default"
		controllerRingName = "operator"
		shardName          = "operator-a"
	)

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()

		fsys = fstest.MapFS{
			"var/run/secrets/kubernetes.io/serviceaccount/namespace": &fstest.MapFile{
				Data: []byte(namespace),
			},
		}
	})

	Describe("#NewResourceLock", func() {
		var (
			restConfig *rest.Config

			options Options
		)

		BeforeEach(func() {
			restConfig = &rest.Config{}

			options = Options{
				ControllerRingName: controllerRingName,
				LeaseNamespace:     "operator-system",
				ShardName:          shardName,
			}
		})

		It("should fail if ControllerRingName is empty", func() {
			options.ControllerRingName = ""

			Expect(NewResourceLock(restConfig, nil, options)).Error().To(MatchError("ControllerRingName is required"))
		})

		It("should use the configured namespace and name", func() {
			resourceLock, err := NewResourceLock(restConfig, nil, options)
			Expect(err).NotTo(HaveOccurred())

			leaseLock := resourceLock.(*LeaseLock)
			Expect(leaseLock.LeaseMeta.Namespace).To(Equal(options.LeaseNamespace))
			Expect(leaseLock.LeaseMeta.Name).To(Equal(options.ShardName))
			Expect(leaseLock.Identity()).To(Equal(leaseLock.LeaseMeta.Name), "identity should equal the shard name")
		})

		It("should default the name to the hostname", func() {
			options.ShardName = ""
			hostname, err := os.Hostname()
			Expect(err).NotTo(HaveOccurred())

			resourceLock, err := NewResourceLock(restConfig, nil, options)
			Expect(err).NotTo(HaveOccurred())

			leaseLock := resourceLock.(*LeaseLock)
			Expect(leaseLock.LeaseMeta.Name).To(Equal(hostname))
			Expect(leaseLock.Identity()).To(Equal(leaseLock.LeaseMeta.Name), "identity should equal the shard name")
		})

		It("should default the namespace to the in-cluster namespace", func() {
			options.LeaseNamespace = ""

			resourceLock, err := NewResourceLock(restConfig, nil, options)
			Expect(err).NotTo(HaveOccurred())

			leaseLock := resourceLock.(*LeaseLock)
			Expect(leaseLock.LeaseMeta.Namespace).To(Equal(namespace))
		})

		It("should fail if the in-cluster namespace cannot be determined", func() {
			options.LeaseNamespace = ""
			fsys = fstest.MapFS{}

			Expect(NewResourceLock(restConfig, nil, options)).Error().To(MatchError(And(
				ContainSubstring("not running in cluster"),
				ContainSubstring("please specify LeaseNamespace"),
			)))
		})
	})

	Describe("#LeaseLock", func() {
		var (
			lock       resourcelock.Interface
			fakeClient coordinationv1client.LeasesGetter

			lease *coordinationv1.Lease
		)

		BeforeEach(func() {
			var err error
			lock, err = NewResourceLock(&rest.Config{}, nil, Options{
				ControllerRingName: controllerRingName,
				LeaseNamespace:     namespace,
				ShardName:          shardName,
			})
			Expect(err).NotTo(HaveOccurred())

			fakeClient = fake.NewClientset().CoordinationV1()
			lock.(*LeaseLock).Client = fakeClient

			lease = &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      shardName,
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity: ptr.To(shardName),
				},
			}
			Expect(fakeClient.Leases(lease.Namespace).Create(ctx, lease, metav1.CreateOptions{})).Error().To(Succeed())
		})

		Describe("#Get", func() {
			It("should return NotFound if the lease does not exist", func() {
				Expect(fakeClient.Leases(lease.Namespace).Delete(ctx, lease.Name, metav1.DeleteOptions{})).To(Succeed())
				Expect(lock.Get(ctx)).Error().To(BeNotFoundError())
			})

			It("should return the existing lease", func() {
				record, _, err := lock.Get(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(record).NotTo(BeNil())
				Expect(record.HolderIdentity).To(Equal(*lease.Spec.HolderIdentity))
			})
		})

		Describe("#Create", func() {
			It("should create the lease if it does not exist", func() {
				Expect(fakeClient.Leases(lease.Namespace).Delete(ctx, lease.Name, metav1.DeleteOptions{})).To(Succeed())

				Expect(lock.Create(ctx, resourcelock.LeaderElectionRecord{
					HolderIdentity: "foo",
				})).To(Succeed())

				Expect(fakeClient.Leases(lease.Namespace).Get(ctx, lease.Name, metav1.GetOptions{})).To(And(
					HaveField("ObjectMeta", And(
						HaveField("Namespace", Equal(namespace)),
						HaveField("Name", Equal(shardName)),
						HaveField("Labels", Equal(map[string]string{
							"alpha.sharding.timebertt.dev/controllerring": controllerRingName,
						})),
					)),
					HaveField("Spec.HolderIdentity", Equal(ptr.To("foo"))),
				))
			})
		})

		Describe("#Update", func() {
			It("should fail if lock is not initialized yet", func() {
				Expect(lock.Update(ctx, resourcelock.LeaderElectionRecord{
					HolderIdentity: "foo",
				})).To(MatchError(ContainSubstring("not initialized")))
			})

			It("should update the lease", func() {
				Expect(lock.Get(ctx)).Error().To(Succeed())

				Expect(lock.Update(ctx, resourcelock.LeaderElectionRecord{
					HolderIdentity: "foo",
				})).To(Succeed())

				Expect(fakeClient.Leases(lease.Namespace).Get(ctx, lease.Name, metav1.GetOptions{})).To(And(
					HaveField("ObjectMeta", And(
						HaveField("Namespace", Equal(namespace)),
						HaveField("Name", Equal(shardName)),
						HaveField("Labels", Equal(map[string]string{
							"alpha.sharding.timebertt.dev/controllerring": controllerRingName,
						})),
					)),
					HaveField("Spec.HolderIdentity", Equal(ptr.To("foo"))),
				))
			})

			It("should keep externally managed labels", func() {
				metav1.SetMetaDataLabel(&lease.ObjectMeta, "foo", "bar")
				Expect(fakeClient.Leases(lease.Namespace).Update(ctx, lease, metav1.UpdateOptions{})).Error().To(Succeed())

				Expect(lock.Get(ctx)).Error().To(Succeed())

				Expect(lock.Update(ctx, resourcelock.LeaderElectionRecord{
					HolderIdentity: "foo",
				})).To(Succeed())

				Expect(fakeClient.Leases(lease.Namespace).Get(ctx, lease.Name, metav1.GetOptions{})).To(
					HaveField("ObjectMeta.Labels", Equal(map[string]string{
						"foo": "bar",
						"alpha.sharding.timebertt.dev/controllerring": controllerRingName,
					})),
				)
			})
		})

		Describe("#RecordEvent", func() {
			Context("no EventRecorder configured", func() {
				It("should do nothing", func() {
					lock.RecordEvent("foo")
				})
			})

			Context("EventRecorder configured", func() {
				var recorder *record.FakeRecorder

				BeforeEach(func() {
					recorder = record.NewFakeRecorder(1)
					lock.(*LeaseLock).LockConfig.EventRecorder = recorder
				})

				It("should send the event", func() {
					Expect(lock.Get(ctx)).Error().To(Succeed())

					lock.RecordEvent("foo")

					Eventually(recorder.Events).Should(Receive(
						Equal("Normal LeaderElection " + shardName + " foo"),
					))
				})
			})
		})

		Describe("#Describe", func() {
			It("should return the lease key", func() {
				Expect(lock.Describe()).To(Equal(client.ObjectKeyFromObject(lease).String()))
			})
		})

		Describe("#Identity()", func() {
			It("should return the lease name", func() {
				Expect(lock.Identity()).To(Equal(lease.Name))
			})
		})
	})

	Describe("#getInClusterNamespace", func() {
		It("should fail because namespace file does not exist", func() {
			fsys = fstest.MapFS{}

			Expect(getInClusterNamespace()).Error().To(MatchError(ContainSubstring("not running in cluster")))
		})

		It("should return content of namespace file", func() {
			Expect(getInClusterNamespace()).To(Equal(namespace))
		})
	})
})
