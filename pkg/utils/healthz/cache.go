/*
Copyright 2023 Tim Ebert.

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

package healthz

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

type cacheSyncWaiter interface {
	WaitForCacheSync(ctx context.Context) bool
}

// CacheSync returns a new healthz.Checker that will pass if all informers in the given cacheSyncWaiter have synced.
func CacheSync(cacheSyncWaiter cacheSyncWaiter) healthz.Checker {
	return func(_ *http.Request) error {
		// cache.Cache.WaitForCacheSync is racy for a closed context, so use context with 5ms timeout instead.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()

		if !cacheSyncWaiter.WaitForCacheSync(ctx) {
			return fmt.Errorf("informers have not synced yet")
		}
		return nil
	}
}
