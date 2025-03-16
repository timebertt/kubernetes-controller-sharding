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
	"net/http"
	"net/url"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	jsonpatchtypes "gomodules.xyz/jsonpatch/v2"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	utilclient "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/client"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/webhook/sharder"
)

var _ = Describe("Handler", func() {
	const controllerRingName = "foo"

	var (
		ctx context.Context

		handler admission.Handler
		metrics *fakeMetrics

		controllerRing *shardingv1alpha1.ControllerRing

		mainObj       *corev1.Secret
		controlledObj *corev1.ConfigMap

		availableShard *coordinationv1.Lease

		fakeClient client.Client
	)

	BeforeEach(func() {
		controllerRing = &shardingv1alpha1.ControllerRing{
			ObjectMeta: metav1.ObjectMeta{
				Name: controllerRingName,
			},
			Spec: shardingv1alpha1.ControllerRingSpec{
				Resources: []shardingv1alpha1.RingResource{{
					GroupResource: metav1.GroupResource{
						Resource: "secrets",
					},
					ControlledResources: []metav1.GroupResource{{
						Resource: "configmaps",
					}},
				}},
			},
		}

		ctx = NewContextWithRequestPath(context.Background(), &http.Request{
			URL: &url.URL{Path: "/webhooks/sharder/controllerring/" + controllerRingName},
		})

		mainObj = &corev1.Secret{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		}
		controlledObj = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: mainObj.Namespace,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: mainObj.APIVersion,
					Kind:       mainObj.Kind,
					Name:       mainObj.Name,
					Controller: ptr.To(true),
				}},
			},
		}

		fakeClock := &clock.RealClock{}
		availableShard = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name: "shard-0",
				Labels: map[string]string{
					shardingv1alpha1.LabelControllerRing: controllerRing.Name,
				},
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       ptr.To("shard-0"),
				LeaseDurationSeconds: ptr.To[int32](10),
				AcquireTime:          ptr.To(metav1.NewMicroTime(fakeClock.Now().Add(-5 * time.Minute))),
				RenewTime:            ptr.To(metav1.NewMicroTime(fakeClock.Now().Add(-2 * time.Second))),
			},
		}

		fakeClient = fake.NewClientBuilder().
			WithScheme(utilclient.SharderScheme).
			WithObjects(controllerRing, availableShard).
			Build()

		metrics = &fakeMetrics{}

		handler = &Handler{
			Reader:  fakeClient,
			Clock:   fakeClock,
			Metrics: metrics,
		}
	})

	Describe("#ControllerRingForRequest", func() {
		It("should return the ControllerRing matching the request", func() {
			Expect(ControllerRingForRequest(ctx, fakeClient)).To(Equal(controllerRing))
		})
	})

	Describe("#Handle", func() {
		When("the request has an invalid path", func() {
			BeforeEach(func() {
				ctx = NewContextWithRequestPath(context.Background(), &http.Request{
					URL: &url.URL{Path: "/webhooks/sharder/controllerring/"},
				})
			})

			It("should return an error", func() {
				Expect(handler.Handle(ctx, newRequest(mainObj))).To(beErrored(ContainSubstring("unexpected request path")))
			})
		})

		When("the request is for a non-existing ControllerRing", func() {
			BeforeEach(func() {
				ctx = NewContextWithRequestPath(context.Background(), &http.Request{
					URL: &url.URL{Path: "/webhooks/sharder/controllerring/not-existing"},
				})
			})

			It("should return an error", func() {
				Expect(handler.Handle(ctx, newRequest(mainObj))).To(beErrored(ContainSubstring("error getting ControllerRing")))
			})
		})

		When("the request has an invalid object", func() {
			It("should return an error", func() {
				req := admission.Request{}
				req.Object.Raw = []byte("foo")

				Expect(handler.Handle(ctx, req)).To(beErrored(ContainSubstring("error decoding object")))
			})
		})

		When("the request has an unrelated object", func() {
			It("should return an error", func() {
				Expect(handler.Handle(ctx, newRequest(&corev1.Pod{}))).To(beErrored(ContainSubstring("not found in ControllerRing")))
			})
		})

		When("the main object uses generateName", func() {
			BeforeEach(func() {
				mainObj.Name = ""
				mainObj.GenerateName = "test-"
			})

			It("should return an error", func() {
				Expect(handler.Handle(ctx, newRequest(mainObj))).To(beErrored(ContainSubstring("generateName is not supported")))
			})
		})

		When("the object is already assigned", func() {
			BeforeEach(func() {
				metav1.SetMetaDataLabel(&mainObj.ObjectMeta, controllerRing.LabelShard(), "foo")
				metav1.SetMetaDataLabel(&controlledObj.ObjectMeta, controllerRing.LabelShard(), "foo")
			})

			It("should do nothing", func() {
				Expect(handler.Handle(ctx, newRequest(mainObj))).To(beAllowed("object is already assigned"))
				Expect(handler.Handle(ctx, newRequest(controlledObj))).To(beAllowed("object is already assigned"))
			})
		})

		When("there is no available shard", func() {
			BeforeEach(func() {
				availableShard.Spec.HolderIdentity = nil
				Expect(fakeClient.Update(ctx, availableShard)).To(Succeed())
			})

			It("should do nothing", func() {
				Expect(handler.Handle(ctx, newRequest(mainObj))).To(beAllowed("there is no available shard"))
				Expect(handler.Handle(ctx, newRequest(controlledObj))).To(beAllowed("there is no available shard"))
			})
		})

		When("the controlled object has no controller reference", func() {
			BeforeEach(func() {
				controlledObj.OwnerReferences = nil
			})

			It("should do nothing", func() {
				Expect(handler.Handle(ctx, newRequest(controlledObj))).To(beAllowed("object should not be assigned"))
			})
		})

		When("the object has no labels", func() {
			It("should add labels and assign the object", func() {
				res := handler.Handle(ctx, newRequest(mainObj))
				Expect(res).To(bePatched())
				Expect(applyPatches(mainObj, res.Patches)).To(HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name))
				Expect(metrics.assignments).To(Equal(1))

				res = handler.Handle(ctx, newRequest(controlledObj))
				Expect(res).To(bePatched())
				Expect(applyPatches(controlledObj, res.Patches)).To(HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name))
				Expect(metrics.assignments).To(Equal(2))
			})
		})

		When("the object has other labels", func() {
			BeforeEach(func() {
				metav1.SetMetaDataLabel(&mainObj.ObjectMeta, "foo", "bar")
				metav1.SetMetaDataLabel(&controlledObj.ObjectMeta, "foo", "bar")
			})

			It("should keep existing labels and assign the object", func() {
				res := handler.Handle(ctx, newRequest(mainObj))
				Expect(res).To(bePatched())
				Expect(applyPatches(mainObj, res.Patches)).To(And(
					HaveLabelWithValue("foo", "bar"),
					HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name),
				))

				res = handler.Handle(ctx, newRequest(controlledObj))
				Expect(res).To(bePatched())
				Expect(applyPatches(controlledObj, res.Patches)).To(And(
					HaveLabelWithValue("foo", "bar"),
					HaveLabelWithValue(controllerRing.LabelShard(), availableShard.Name),
				))
			})
		})

		When("the request is dry-run", func() {
			It("should not observe metrics", func() {
				req := newRequest(mainObj)
				req.DryRun = ptr.To(true)
				Expect(handler.Handle(ctx, req)).To(bePatched())
				Expect(metrics.assignments).To(BeZero())
			})
		})
	})
})

func newRequest(obj client.Object) admission.Request {
	GinkgoHelper()

	var gvr metav1.GroupVersionResource
	switch obj.(type) {
	case *corev1.Secret:
		gvr = metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	case *corev1.ConfigMap:
		gvr = metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	case *corev1.Pod:
		gvr = metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	default:
		Fail("unknown object type")
	}

	req := admission.Request{}
	req.Resource = gvr

	var err error
	req.Object.Raw, err = json.Marshal(obj)
	Expect(err).NotTo(HaveOccurred())

	return req
}

func beAllowed(message any) gomegatypes.GomegaMatcher {
	return And(
		HaveField("Patches", BeEmpty()),
		HaveField("AdmissionResponse", And(
			HaveField("Allowed", BeTrue()),
			HaveField("Result.Message", message),
		)),
	)
}

func bePatched() gomegatypes.GomegaMatcher {
	return And(
		HaveField("Patches", Not(BeEmpty())),
		HaveField("AdmissionResponse", And(
			HaveField("Allowed", BeTrue()),
			HaveField("Result.Message", "assigning object"),
		)),
	)
}

func beErrored(message any) gomegatypes.GomegaMatcher {
	return And(
		HaveField("Patches", BeEmpty()),
		HaveField("AdmissionResponse", And(
			HaveField("Allowed", BeFalse()),
			HaveField("Result.Message", message),
		)),
	)
}

func applyPatches[T client.Object](obj T, patches []jsonpatchtypes.Operation) T {
	GinkgoHelper()

	rawObj, err := json.Marshal(obj)
	Expect(err).NotTo(HaveOccurred())

	rawPatch, err := json.Marshal(patches)
	Expect(err).NotTo(HaveOccurred())

	patch, err := jsonpatch.DecodePatch(rawPatch)
	Expect(err).NotTo(HaveOccurred())

	rawPatchedObj, err := patch.Apply(rawObj)
	Expect(err).NotTo(HaveOccurred())

	patchedObj := obj.DeepCopyObject().(T)
	Expect(json.Unmarshal(rawPatchedObj, patchedObj)).To(Succeed())

	return patchedObj
}

type fakeMetrics struct {
	assignments int
}

func (f *fakeMetrics) ObserveAssignment(string, metav1.GroupResource) {
	f.assignments++
}
