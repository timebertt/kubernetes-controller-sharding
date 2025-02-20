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

package pager_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/pager"
	. "github.com/timebertt/kubernetes-controller-sharding/pkg/utils/test/matchers"
)

var _ = Describe("ListPager", func() {
	const (
		pageSize    = 2
		pageCount   = 5
		objectCount = 9
	)

	var (
		ctx context.Context

		allPods []corev1.Pod

		reader *lister
		pager  *ListPager
	)

	BeforeEach(func() {
		ctx = context.Background()

		allPods = podSlice(0, objectCount)

		reader = &lister{allPods: allPods}

		pager = New(reader)
		pager.PageSize = pageSize
		pager.PageBufferSize = 2
	})

	It("should page all objects", func() {
		var pods []*corev1.Pod

		Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
			pods = append(pods, obj.(*corev1.Pod))
			return nil
		})).To(Succeed())

		Expect(pods).To(havePods(1, objectCount))

		Expect(reader.calls).To(Equal(pageCount))
	})

	It("should page objects with the custom limit", func() {
		var pods []client.Object

		i := 0
		Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
			if obj != &allPods[i] {
				return fmt.Errorf("the pager should reuse the object")
			}
			i++
			pods = append(pods, obj)
			return nil
		}, client.Limit(objectCount+1))).To(Succeed())

		Expect(pods).To(havePods(1, objectCount))

		Expect(reader.calls).To(Equal(1))
	})

	It("should fail due to negative page size", func() {
		pager.PageSize = -1
		Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
			return nil
		})).To(MatchError(ContainSubstring("PageSize must be >= 0")))
	})

	It("should fail due to negative page buffer size", func() {
		pager.PageBufferSize = -1
		Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
			return nil
		})).To(MatchError(ContainSubstring("PageBufferSize must be >= 0")))
	})

	It("should return the lister error", func() {
		Expect(pager.EachListItem(ctx, &corev1.ConfigMapList{}, func(obj client.Object) error {
			return nil
		})).To(MatchError("expected *corev1.PodList, got *v1.ConfigMapList"))
	})

	It("should return the iterator error", func() {
		Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
			if obj.GetName() == "pod-5" {
				return fmt.Errorf("foo")
			}
			return nil
		})).To(MatchError("foo"))
	})

	It("should cancel the operation when the context is canceled", func(specCtx SpecContext) {
		done := make(chan struct{})

		blockConsumer := make(chan struct{})
		defer close(blockConsumer)

		ctx, cancel := context.WithCancel(specCtx)
		go func() {
			defer GinkgoRecover()

			Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
				<-blockConsumer
				return nil
			})).To(MatchError(context.Canceled))

			close(done)
		}()

		cancel()
		Eventually(specCtx, done).Should(BeClosed())
	}, SpecTimeout(time.Second))

	It("should buffer the configured number of pages", func(specCtx SpecContext) {
		done := make(chan struct{})

		ctx, cancel := context.WithCancel(specCtx)
		go func() {
			defer GinkgoRecover()

			Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
				<-ctx.Done()
				return nil
			})).To(MatchError(context.Canceled))

			close(done)
		}()

		// consumer takes one chunk out and one chunk is produced but blocked,
		// so we have made PageBufferSize + 2 calls to the lister
		Eventually(specCtx, reader.getCalls).Should(BeEquivalentTo(pager.PageBufferSize + 2))

		cancel()
		Eventually(specCtx, done).Should(BeClosed())
	}, SpecTimeout(time.Second))

	It("should correctly handle the resourceVersion fields", func() {
		Expect(pager.EachListItem(ctx, &corev1.PodList{}, func(obj client.Object) error {
			return nil
		}, &client.ListOptions{Raw: &metav1.ListOptions{
			ResourceVersion:      "0",
			ResourceVersionMatch: "NotOlderThan",
		}})).To(Succeed())
	})

	Describe("#EachListItemWithAlloc", func() {
		It("should page all objects", func() {
			var pods []client.Object

			i := 0
			Expect(pager.EachListItemWithAlloc(ctx, &corev1.PodList{}, func(obj client.Object) error {
				if obj == &allPods[i] {
					return fmt.Errorf("the pager should copy the object")
				}
				i++
				pods = append(pods, obj)
				return nil
			})).To(Succeed())

			Expect(pods).To(havePods(1, objectCount))

			Expect(reader.calls).To(Equal(pageCount))
		})
	})
})

type lister struct {
	mu    sync.Mutex
	calls int

	allPods      []corev1.Pod
	previousList client.ObjectList
}

func (l *lister) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.calls++
	}()

	if list == l.previousList {
		return fmt.Errorf("the pager should not reuse the list for multiple calls")
	}
	l.previousList = list

	podList, ok := list.(*corev1.PodList)
	if !ok {
		return fmt.Errorf("expected *corev1.PodList, got %T", list)
	}

	listOptions := &client.ListOptions{}
	listOptions.ApplyOptions(opts)

	if l.calls > 1 {
		if listOptions.Raw.ResourceVersion != "" {
			return fmt.Errorf("the pager should reset the resourceVersion field for consecutive calls")
		}
		if listOptions.Raw.ResourceVersionMatch != "" {
			return fmt.Errorf("the pager should reset the resourceVersionMatch field for consecutive calls")
		}
	}

	limit := listOptions.Limit
	if limit == 0 {
		return fmt.Errorf("the pager should set limit")
	}

	var offset int64
	if listOptions.Continue != "" {
		var err error
		offset, err = strconv.ParseInt(listOptions.Continue, 10, 64)
		if err != nil {
			return err
		}
	}

	defer func() {
		if offset+limit >= int64(len(l.allPods)) {
			podList.Continue = ""
		} else {
			podList.Continue = strconv.FormatInt(offset+int64(len(podList.Items)), 10)
		}
	}()

	podList.Items = l.allPods[offset:min(int64(len(l.allPods)), offset+limit)]
	return nil
}

func (l *lister) getCalls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func podSlice(offset, n int64) []corev1.Pod {
	pods := make([]corev1.Pod, n)

	for i := int64(0); i < n; i++ {
		pods[i] = corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-" + strconv.FormatInt(offset+i+1, 10)}}
	}

	return pods
}

func havePods(i, j int) gomegatypes.GomegaMatcher {
	var matchers []any

	for ; i <= j; i++ {
		matchers = append(matchers, HaveName("pod-"+strconv.Itoa(i)))
	}

	return HaveExactElements(matchers...)
}
