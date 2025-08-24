/*
Copyright 2022 Tim Ebert.

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

package generator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log

// EmitN returns a source that emits exactly n reconcile requests with the given delay.
// Use it with the controller builder:
//
//	WatchesRawSource(EmitN(n, time.Second))
//
// Or a plain controller:
//
//	Watch(EmitN(n, time.Second))
func EmitN(n int, delay time.Duration) source.Source {
	return source.TypedFunc[reconcile.Request](func(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
		for i := 0; i < n; i++ {
			queue.AddAfter(reconcile.Request{NamespacedName: types.NamespacedName{
				// use different object names, otherwise queue will merge the requests
				Name: fmt.Sprintf("request-%d", n),
			}}, delay)
		}

		return nil
	})
}

// StopOnContextCanceled wraps the given reconciler so that "context canceled" errors are ignored. This is helpful when
// a reconciler is expected to be canceled (e.g., when the scenario finishes). We neither need to retry nor log on such
// errors. We can just stop silently.
func StopOnContextCanceled(r reconcile.Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
		result, err := r.Reconcile(ctx, request)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		return result, err
	})
}

// NTimesConcurrently runs the given action n times. It distributes the work across the given number of concurrent
// workers.
func NTimesConcurrently(n, workers int, do func() error) error {
	var (
		wg   sync.WaitGroup
		work = make(chan struct{}, workers)
		errs = make(chan error, workers)
	)

	// start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range work {
				errs <- do()
			}
		}()
	}

	// collect all errors
	var (
		allErrs  *multierror.Error
		errsDone = make(chan struct{})
	)
	go func() {
		for err := range errs {
			allErrs = multierror.Append(allErrs, err)
		}
		close(errsDone)
	}()

	// emit n work items
	for i := 0; i < n; i++ {
		work <- struct{}{}
	}
	close(work)

	// wait for all workers to process all work items
	wg.Wait()
	// signal error worker and wait for it to process all errors
	close(errs)
	<-errsDone

	return allErrs.ErrorOrNil()
}

// RetryOnError runs the given action with a short timeout and retries it up to `retries` times if it retruns a
// retriable error.
// This is useful when creating a lot of objects in parallel with generateName which can lead to AlreadyExists errors.
func RetryOnError(ctx context.Context, retries int, do func(context.Context) error, retriable func(error) bool) error {
	i := 0
	return wait.PollUntilContextTimeout(ctx, 50*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (done bool, err error) {
		err = do(ctx)
		if err == nil {
			return true, nil
		}

		// stop retrying on non-retriable errors
		if !retriable(err) {
			return true, err
		}

		// stop retrying when reaching the max retry count
		i++
		if i > retries {
			return true, err
		}

		// otherwise, retry another time
		return false, nil
	})
}

// CreateClusterScopedOwnerObject creates a new cluster-scoped object that has a single purpose: being used as an owner
// for multiple objects that should be cleaned up at once. This is useful for cleaning up a lot of objects (of different
// kinds) at once with a single DELETE call.
func CreateClusterScopedOwnerObject(ctx context.Context, c client.Client, opts ...GenerateOption) (client.Object, *metav1.OwnerReference, error) {
	options := (&GenerateOptions{}).ApplyOptions(opts...)

	ownerObject := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "experiment-owner-",
		},
	}
	options.ApplyToObject(&ownerObject.ObjectMeta)

	if err := c.Create(ctx, ownerObject); err != nil {
		return nil, nil, err
	}

	return ownerObject, metav1.NewControllerRef(ownerObject, rbacv1.SchemeGroupVersion.WithKind("ClusterRole")), nil
}
