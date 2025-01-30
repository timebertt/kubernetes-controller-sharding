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

package controllerring

import (
	"context"
	"fmt"
	"maps"
	"path"
	"strings"

	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/config/v1alpha1"
	shardingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/pkg/apis/sharding/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/sharding/leases"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/utils"
	"github.com/timebertt/kubernetes-controller-sharding/pkg/webhook/sharder"
)

//+kubebuilder:rbac:groups=sharding.timebertt.dev,resources=controllerrings,verbs=get;list;watch
//+kubebuilder:rbac:groups=sharding.timebertt.dev,resources=controllerrings/status,verbs=update;patch
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconciler reconciles ControllerRings.
type Reconciler struct {
	Client   client.Client
	Recorder record.EventRecorder
	Clock    clock.PassiveClock
	Config   *configv1alpha1.SharderConfig
}

// Reconcile reconciles a ControllerRing object.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	controllerRing := &shardingv1alpha1.ControllerRing{}
	if err := r.Client.Get(ctx, req.NamespacedName, controllerRing); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	before := controllerRing.DeepCopy()

	// reconcile sharder webhook configs
	if err := r.reconcileWebhooks(ctx, controllerRing); err != nil {
		return reconcile.Result{}, r.updateStatusError(ctx, log, fmt.Errorf("error reconciling webhooks for ControllerRing: %w", err), controllerRing, before)
	}

	// collect list of shards in the ring
	leaseList := &coordinationv1.LeaseList{}
	if err := r.Client.List(ctx, leaseList, client.MatchingLabelsSelector{Selector: controllerRing.LeaseSelector()}); err != nil {
		return reconcile.Result{}, r.updateStatusError(ctx, log, fmt.Errorf("error listing Leases for ControllerRing: %w", err), controllerRing, before)
	}

	shards := leases.ToShards(leaseList.Items, r.Clock.Now())
	controllerRing.Status.Shards = int32(len(shards))                            // nolint:gosec
	controllerRing.Status.AvailableShards = int32(len(shards.AvailableShards())) // nolint:gosec

	// update status if necessary
	return reconcile.Result{}, r.updateStatusSuccess(ctx, controllerRing, before)
}

func (r *Reconciler) updateStatusSuccess(ctx context.Context, controllerRing, before *shardingv1alpha1.ControllerRing) error {
	if err := r.OptionallyUpdateStatus(ctx, controllerRing, before, func(ready *metav1.Condition) {
		ready.Status = metav1.ConditionTrue
		ready.Reason = "ReconciliationSucceeded"
		ready.Message = "ControllerRing was successfully reconciled"
	}); err != nil {
		return fmt.Errorf("error updating ControllerRing status: %w", err)
	}
	return nil
}

func (r *Reconciler) updateStatusError(ctx context.Context, log logr.Logger, reconcileError error, controllerRing, before *shardingv1alpha1.ControllerRing) error {
	message := utils.CapitalizeFirst(reconcileError.Error())

	r.Recorder.Event(controllerRing, corev1.EventTypeWarning, "ReconciliationFailed", message)

	if err := r.OptionallyUpdateStatus(ctx, controllerRing, before, func(ready *metav1.Condition) {
		ready.Status = metav1.ConditionFalse
		ready.Reason = "ReconciliationFailed"
		ready.Message = message
	}); err != nil {
		// We will return the underlying error to the controller. If we fail to publish it to the status, make sure to log
		// it at least.
		log.Error(err, "Error updating ControllerRing status with error")
	}

	return reconcileError
}

func (r *Reconciler) OptionallyUpdateStatus(ctx context.Context, controllerRing, before *shardingv1alpha1.ControllerRing, mutate func(ready *metav1.Condition)) error {
	// always update status with the latest observed generation, no matter if reconciliation succeeded or not
	controllerRing.Status.ObservedGeneration = controllerRing.Generation
	readyCondition := metav1.Condition{
		Type:               shardingv1alpha1.ControllerRingReady,
		ObservedGeneration: controllerRing.Generation,
	}

	mutate(&readyCondition)
	meta.SetStatusCondition(&controllerRing.Status.Conditions, readyCondition)

	if apiequality.Semantic.DeepEqual(controllerRing.Status, before.Status) {
		return nil
	}

	return r.Client.Status().Update(ctx, controllerRing)
}

func (r *Reconciler) reconcileWebhooks(ctx context.Context, controllerRing *shardingv1alpha1.ControllerRing) error {
	webhookConfig, err := r.WebhookConfigForControllerRing(controllerRing)
	if err != nil {
		return err
	}

	return r.Client.Patch(ctx, webhookConfig, client.Apply)
}

func (r *Reconciler) WebhookConfigForControllerRing(controllerRing *shardingv1alpha1.ControllerRing) (*admissionregistrationv1.MutatingWebhookConfiguration, error) {
	webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: admissionregistrationv1.SchemeGroupVersion.String(),
			Kind:       "MutatingWebhookConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "controllerring-" + controllerRing.Name,
			Labels: map[string]string{
				"app.kubernetes.io/name":             shardingv1alpha1.AppControllerSharding,
				shardingv1alpha1.LabelControllerRing: controllerRing.Name,
			},
			Annotations: maps.Clone(r.Config.Webhook.Config.Annotations),
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{r.WebhookForControllerRing(controllerRing)},
	}
	if err := controllerutil.SetControllerReference(controllerRing, webhookConfig, r.Client.Scheme()); err != nil {
		return nil, fmt.Errorf("error setting controller reference: %w", err)
	}

	return webhookConfig, nil
}

func (r *Reconciler) WebhookForControllerRing(controllerRing *shardingv1alpha1.ControllerRing) admissionregistrationv1.MutatingWebhook {
	webhook := admissionregistrationv1.MutatingWebhook{
		Name:              "sharder.sharding.timebertt.dev",
		ClientConfig:      *r.Config.Webhook.Config.ClientConfig.DeepCopy(),
		NamespaceSelector: r.Config.Webhook.Config.NamespaceSelector.DeepCopy(),

		// only process unassigned objects
		ObjectSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      controllerRing.LabelShard(),
				Operator: metav1.LabelSelectorOpDoesNotExist,
			}},
		},

		// Choose Ignore to trade immediate assignments for minimal disruption, the sharder controller will assign any
		// objects that missed the sharder webhook.
		FailurePolicy:           ptr.To(admissionregistrationv1.Ignore),
		TimeoutSeconds:          ptr.To(int32(5)),
		SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
		AdmissionReviewVersions: []string{"v1"},
	}

	// overwrite namespaceSelector with ring-specific namespaceSelector if specified
	if controllerRing.Spec.NamespaceSelector != nil {
		webhook.NamespaceSelector = controllerRing.Spec.NamespaceSelector.DeepCopy()
	}

	// add ring-specific path to webhook client config
	webhookPath := sharder.WebhookPathForControllerRing(controllerRing)

	if service := webhook.ClientConfig.Service; service != nil {
		service.Path = ptr.To(path.Join(ptr.Deref(service.Path, ""), webhookPath))
	}
	if url := webhook.ClientConfig.URL; url != nil {
		// We can't use path.Join on URLs because it will drop one slash from the scheme.
		// We accept both URLs with and without trailing slashes, so trim it if present to ensure we have only one as the
		// path separator.
		*url = strings.TrimSuffix(*url, "/") + webhookPath
	}

	// add rules for all ring resources
	for _, ringResource := range controllerRing.Spec.Resources {
		webhook.Rules = append(webhook.Rules, RuleForResource(ringResource.GroupResource))

		for _, controlledResource := range ringResource.ControlledResources {
			webhook.Rules = append(webhook.Rules, RuleForResource(controlledResource))
		}
	}

	return webhook
}

// RuleForResource returns the sharder's webhook rule for the given resource.
func RuleForResource(gr metav1.GroupResource) admissionregistrationv1.RuleWithOperations {
	return admissionregistrationv1.RuleWithOperations{
		Operations: []admissionregistrationv1.OperationType{
			admissionregistrationv1.Create,
			admissionregistrationv1.Update,
		},
		Rule: admissionregistrationv1.Rule{
			APIGroups:   []string{gr.Group},
			APIVersions: []string{"*"},
			Resources:   []string{gr.Resource},
			Scope:       ptr.To(admissionregistrationv1.AllScopes),
		},
	}
}
