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

package webhosting

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SilenceConflicts wraps a reconciler to not return conflict errors. The requests is requeued with exponential backoff
// as if an error was returned but the error will not be logged.
func SilenceConflicts(r reconcile.Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
		result, err := r.Reconcile(ctx, request)
		if apierrors.IsConflict(err) {
			// nolint:staticcheck // this should be a valid use case of Result.Requeue
			result.Requeue = true
			// RequeueAfter takes precedence over Requeue, set it to zero in case it was returned alongside a conflict error
			result.RequeueAfter = 0
			err = nil
		}
		return result, err
	})
}
