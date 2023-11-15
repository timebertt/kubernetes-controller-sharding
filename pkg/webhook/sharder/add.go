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

package sharder

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding"
)

const (
	// HandlerName is the name of the webhook handler.
	HandlerName = "sharder"
	// WebhookPathPrefix is the path prefix at which the handler should be registered.
	WebhookPathPrefix = "/webhooks/sharder/"
)

// AddToManager adds Handler to the given manager.
func (h *Handler) AddToManager(mgr manager.Manager) error {
	if h.Reader == nil {
		h.Reader = mgr.GetCache()
	}

	mgr.GetWebhookServer().Register(WebhookPathPrefix, &admission.Webhook{
		Handler:         h,
		WithContextFunc: NewContextWithRequestPath,
		RecoverPanic:    true,
	})
	return nil
}

const pathClusterRing = "clusterring"

// WebhookPathFor returns the webhook handler path that should be used for implementing the given ring object.
// It is the reverse of RingForWebhookPath.
func WebhookPathFor(obj client.Object) (string, error) {
	switch obj.(type) {
	case *shardingv1alpha1.ClusterRing:
		return path.Join(WebhookPathPrefix, pathClusterRing, obj.GetName()), nil
	default:
		return "", fmt.Errorf("unexpected kind %T", obj)
	}
}

// RingForWebhookPath returns the ring object that is associated with the given webhook handler path.
// It is the reverse of WebhookPathFor.
func RingForWebhookPath(requestPath string) (sharding.Ring, error) {
	if !strings.HasPrefix(requestPath, WebhookPathPrefix) {
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}

	parts := strings.SplitN(strings.TrimPrefix(requestPath, WebhookPathPrefix), "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}

	var ring sharding.Ring
	switch parts[0] {
	case pathClusterRing:
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected request path: %s", requestPath)
		}
		ring = &shardingv1alpha1.ClusterRing{ObjectMeta: metav1.ObjectMeta{Name: parts[1]}}
	default:
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}

	return ring, nil
}

type ctxKey int

// keyRequestPath is used for storing the admission request's path in contexts.
const keyRequestPath ctxKey = 0

// NewContextWithRequestPath augments the given context with the request's path.
func NewContextWithRequestPath(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, keyRequestPath, r.URL.Path)
}

// RequestPathFromContext returns the request's path stored in the given context.
func RequestPathFromContext(ctx context.Context) (string, error) {
	if v, ok := ctx.Value(keyRequestPath).(string); ok {
		return v, nil
	}

	return "", fmt.Errorf("no request path found in context")
}
