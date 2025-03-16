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
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
)

// webhookPathPrefix is the path prefix at which the handler should be registered.
const webhookPathPrefix = "/webhooks/sharder/"

// AddToManager adds Handler to the given manager.
func (h *Handler) AddToManager(mgr manager.Manager) error {
	if h.Reader == nil {
		h.Reader = mgr.GetCache()
	}
	if h.Clock == nil {
		h.Clock = clock.RealClock{}
	}

	mgr.GetWebhookServer().Register(webhookPathPrefix, &admission.Webhook{
		Handler:         h,
		WithContextFunc: NewContextWithRequestPath,
	})
	return nil
}

const pathControllerRing = "controllerring"

// WebhookPathForControllerRing returns the webhook handler path that should be used for implementing the given
// ControllerRing. It is the reverse of ControllerRingForWebhookPath.
func WebhookPathForControllerRing(ring *shardingv1alpha1.ControllerRing) string {
	return path.Join(webhookPathPrefix, pathControllerRing, ring.Name)
}

// ControllerRingForWebhookPath returns the ControllerRing that is associated with the given webhook handler path.
// It is the reverse of WebhookPathForControllerRing.
func ControllerRingForWebhookPath(requestPath string) (*shardingv1alpha1.ControllerRing, error) {
	if !strings.HasPrefix(requestPath, webhookPathPrefix) {
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}

	parts := strings.SplitN(strings.TrimPrefix(requestPath, webhookPathPrefix), "/", 3)
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}
	if parts[0] != pathControllerRing {
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}
	if parts[1] == "" {
		return nil, fmt.Errorf("unexpected request path: %s", requestPath)
	}

	return &shardingv1alpha1.ControllerRing{ObjectMeta: metav1.ObjectMeta{Name: parts[1]}}, nil
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
