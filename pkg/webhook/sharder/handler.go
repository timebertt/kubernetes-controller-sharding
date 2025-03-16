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
	"strings"

	"gomodules.xyz/jsonpatch/v2"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/key"
	shardingmetrics "github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/metrics"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/ring"
)

// Handler handles admission requests and invalidates the static token in Secret resources related to ServiceAccounts.
type Handler struct {
	Reader client.Reader
	Clock  clock.PassiveClock
}

func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := logf.FromContext(ctx)

	controllerRing, err := ControllerRingForRequest(ctx, h.Reader)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error determining ControllerRing for request: %w", err))
	}

	// Unfortunately, admission.Decoder / runtime.Decoder can't handle decoding into PartialObjectMetadata.
	// We are only interested in the metadata and webhooks always use json encoding, so let's simply decode ourselves.
	obj := &metav1.PartialObjectMetadata{}
	if err := json.Unmarshal(req.Object.Raw, obj); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error decoding object: %w", err))
	}

	labelShard := controllerRing.LabelShard()

	// Don't touch labels that the object already has, we can't simply reassign it because the active shard might still
	// be working on it.
	if obj.Labels[labelShard] != "" {
		return admission.Allowed("object is already assigned")
	}

	keyFunc, err := key.FuncForResource(metav1.GroupResource{
		Group:    req.Resource.Group,
		Resource: req.Resource.Resource,
	}, controllerRing)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("error determining hash key func for object: %w", err))
	}

	hashKey, err := keyFunc(obj)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("error calculating hash key for object: %w", err))
	}
	if hashKey == "" {
		return admission.Allowed("object should not be assigned")
	}

	// collect list of shards in the ring
	leaseList := &coordinationv1.LeaseList{}
	if err := h.Reader.List(ctx, leaseList, client.MatchingLabelsSelector{Selector: controllerRing.LeaseSelector()}); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("error listing Leases for ControllerRing: %w", err))
	}

	// get ring from cache and hash the object onto the ring
	hashRing, _ := ring.FromLeases(controllerRing, leaseList, h.Clock.Now())
	shard := hashRing.Hash(hashKey)
	if shard == "" {
		return admission.Allowed("there is no available shard")
	}

	log.V(1).Info("Assigning object for ControllerRing", "controllerRing", client.ObjectKeyFromObject(controllerRing), "shard", shard)

	patches := make([]jsonpatch.JsonPatchOperation, 0, 2)
	if obj.Labels == nil {
		patches = append(patches, jsonpatch.NewOperation("add", "/metadata/labels", map[string]string{}))
	}
	// If we reach here, the shard label is always missing. Otherwise, we would have exited early.
	patches = append(patches, jsonpatch.NewOperation("add", "/metadata/labels/"+rfc6901Encoder.Replace(labelShard), shard))

	if !ptr.Deref(req.DryRun, false) {
		shardingmetrics.AssignmentsTotal.WithLabelValues(
			controllerRing.Name, req.Resource.Group, req.Resource.Resource,
		).Inc()
	}

	return admission.Patched("assigning object", patches...)
}

// ControllerRingForRequest returns the Ring object matching the requests' path.
func ControllerRingForRequest(ctx context.Context, c client.Reader) (*shardingv1alpha1.ControllerRing, error) {
	requestPath, err := RequestPathFromContext(ctx)
	if err != nil {
		return nil, err
	}

	controllerRing, err := ControllerRingForWebhookPath(requestPath)
	if err != nil {
		return nil, err
	}

	if err := c.Get(ctx, client.ObjectKeyFromObject(controllerRing), controllerRing); err != nil {
		return nil, fmt.Errorf("error getting ControllerRing: %w", err)
	}

	return controllerRing, nil
}

// rfc6901Encoder can escape / characters in label keys for inclusion in JSON patch paths.
var rfc6901Encoder = strings.NewReplacer("~", "~0", "/", "~1")
