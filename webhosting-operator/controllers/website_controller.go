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

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/api/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/templates"
)

// WebsiteReconciler reconciles a Website object.
type WebsiteReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites/finalizers,verbs=update

// Reconcile reconciles a Website object.
func (r *WebsiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling website")

	website := &webhostingv1alpha1.Website{}
	if err := r.Get(ctx, req.NamespacedName, website); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Object is gone, stop reconciling")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}
	// update status with the latest observed generation
	website.Status.ObservedGeneration = website.Generation

	if website.Spec.Theme == "" {
		log.Error(fmt.Errorf("website doesn't specify a theme"), "Unable to reconcile Website")
		r.Recorder.Event(website, corev1.EventTypeWarning, "ThemeUnspecified", "Website doesn't specify a Theme")

		website.Status.Phase = webhostingv1alpha1.PhaseError
		// Only requeue with backoff if we fail to update the status. We can't do much till the spec changes, so rather wait
		// for the next update event.
		return ctrl.Result{}, r.Status().Update(ctx, website)
	}

	// retrieve theme
	log = log.WithValues("theme", website.Spec.Theme)
	theme := &webhostingv1alpha1.Theme{}
	if err := r.Get(ctx, client.ObjectKey{Name: website.Spec.Theme}, theme); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ThemeNotFound", "Theme %s not found", website.Spec.Theme)
		}
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error getting Theme %s: %v", website.Spec.Theme, err)
	}

	downstreamName := calculateDownstreamName(website)

	// get current deployment status
	currentDeployment := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: website.Namespace, Name: downstreamName}, currentDeployment); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error getting Deployment: %v", err)
	}

	// create downstream objects
	configMap, err := r.ConfigMapForWebsite(downstreamName, website, theme)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing ConfigMap: %v", err)
	}
	if err := r.Patch(ctx, configMap, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying ConfigMap: %v", err)
	}

	service, err := r.ServiceForWebsite(downstreamName, website)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing Service: %v", err)
	}
	if err := r.Patch(ctx, service, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying Service: %v", err)
	}

	deployment, err := r.DeploymentForWebsite(downstreamName, website)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing Deployment: %v", err)
	}
	if err := r.Patch(ctx, deployment, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying Deployment: %v", err)
	}

	// update status
	newPhase := webhostingv1alpha1.PhasePending
	if cond := GetDeploymentCondition(currentDeployment.Status.Conditions, appsv1.DeploymentAvailable); cond != nil && cond.Status == corev1.ConditionTrue {
		newPhase = webhostingv1alpha1.PhaseReady
	}
	website.Status.Phase = newPhase

	return ctrl.Result{}, r.Status().Update(ctx, website)
}

func (r *WebsiteReconciler) recordErrorAndUpdateStatus(ctx context.Context, website *webhostingv1alpha1.Website, reason, messageFmt string, args ...interface{}) error {
	r.Recorder.Eventf(website, corev1.EventTypeWarning, reason, messageFmt, args...)

	website.Status.Phase = webhostingv1alpha1.PhaseError
	if err := r.Status().Update(ctx, website); err != nil {
		// unable to update status, requeue with backoff
		return err
	}
	// return error to retry with backoff
	return fmt.Errorf(messageFmt, args...)
}

func (r *WebsiteReconciler) ConfigMapForWebsite(name string, website *webhostingv1alpha1.Website, theme *webhostingv1alpha1.Theme) (*corev1.ConfigMap, error) {
	indexHTML, err := templates.RenderIndexHTML(name, website, theme)
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: website.Namespace,
			Labels:    labels(name),
		},
		Data: map[string]string{
			"index.html": indexHTML,
		},
	}

	return configMap, ctrl.SetControllerReference(website, configMap, r.Scheme)
}

func (r *WebsiteReconciler) ServiceForWebsite(name string, website *webhostingv1alpha1.Website) (*corev1.Service, error) {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: website.Namespace,
			Labels:    labels(name),
		},
		Spec: corev1.ServiceSpec{
			Selector: labels(name),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	return service, ctrl.SetControllerReference(website, service, r.Scheme)
}

func (r *WebsiteReconciler) DeploymentForWebsite(name string, website *webhostingv1alpha1.Website) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: website.Namespace,
			Labels:    labels(name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels(name),
			},
			Replicas:             pointer.Int32(1),
			RevisionHistoryLimit: pointer.Int32(2),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels(name),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.21-alpine",
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: 80,
							Protocol:      corev1.ProtocolTCP,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "website-data",
							ReadOnly:  true,
							MountPath: "/usr/share/nginx/html",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "website-data",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: name},
							},
						},
					}},
				},
			},
		},
	}

	return deployment, ctrl.SetControllerReference(website, deployment, r.Scheme)
}

func labels(name string) map[string]string {
	return map[string]string{
		"app":        "website",
		"website":    name,
		"managed-by": "webhosting-operator",
	}
}

func calculateDownstreamName(website *webhostingv1alpha1.Website) string {
	// Customers might delete the website and create a new one with the same name.
	// To avoid clashes in that case, we need to include the website's UID in the name of owned objects.
	// Take a sha256 sum and include the first 6 hex characters.
	checksum := sha256.Sum256([]byte(website.UID))
	return website.Name + "-" + hex.EncodeToString(checksum[:])[:6]
}

const websiteThemeField = ".spec.theme"

// SetupWithManager sets up the controller with the Manager.
func (r *WebsiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &webhostingv1alpha1.Website{}, websiteThemeField, func(obj client.Object) []string {
		website := obj.(*webhostingv1alpha1.Website)
		return []string{website.Spec.Theme}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&webhostingv1alpha1.Website{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// watch deployments in order to update phase on relevant changes
		Owns(&appsv1.Deployment{}, builder.WithPredicates(DeploymentConditionsChanged)).
		// watch owned objects for relevant changes to reconcile them back if changed
		Owns(&corev1.ConfigMap{}, builder.WithPredicates(ConfigMapDataChanged)).
		Owns(&corev1.Service{}, builder.WithPredicates(ServiceSpecChanged)).
		// watch themes to rollout theme changes to all referencing websites
		Watches(
			&source.Kind{Type: &webhostingv1alpha1.Theme{}},
			handler.EnqueueRequestsFromMapFunc(r.MapThemeToWebsites),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}

// MapThemeToWebsites maps a theme to all websites that use it.
func (r *WebsiteReconciler) MapThemeToWebsites(theme client.Object) []reconcile.Request {
	websiteList := &webhostingv1alpha1.WebsiteList{}
	err := r.List(context.TODO(), websiteList, client.MatchingFields{websiteThemeField: theme.GetName()})
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(websiteList.Items))
	for i, website := range websiteList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      website.GetName(),
				Namespace: website.GetNamespace(),
			},
		}
	}
	return requests
}

// DeploymentConditionsChanged is a predicate for filtering relevant Deployment events.
var DeploymentConditionsChanged = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}

		oldDeployment, ok := e.ObjectOld.(*appsv1.Deployment)
		if !ok {
			return false
		}
		newDeployment, ok := e.ObjectNew.(*appsv1.Deployment)
		if !ok {
			return false
		}

		oldAvailable := GetDeploymentCondition(oldDeployment.Status.Conditions, appsv1.DeploymentAvailable)
		newAvailable := GetDeploymentCondition(newDeployment.Status.Conditions, appsv1.DeploymentAvailable)
		return !apiequality.Semantic.DeepEqual(oldAvailable, newAvailable)
	},
}

// ConfigMapDataChanged is a predicate for filtering relevant ConfigMap events.
var ConfigMapDataChanged = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}

		oldConfigMap, ok := e.ObjectOld.(*corev1.ConfigMap)
		if !ok {
			return false
		}
		newConfigMap, ok := e.ObjectNew.(*corev1.ConfigMap)
		if !ok {
			return false
		}
		return !apiequality.Semantic.DeepEqual(oldConfigMap.Data, newConfigMap.Data)
	},
}

// ServiceSpecChanged is a predicate for filtering relevant Service events.
var ServiceSpecChanged = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}

		oldService, ok := e.ObjectOld.(*corev1.Service)
		if !ok {
			return false
		}
		newService, ok := e.ObjectNew.(*corev1.Service)
		if !ok {
			return false
		}
		return !apiequality.Semantic.DeepEqual(oldService.Spec, newService.Spec)
	},
}

// GetDeploymentCondition returns the condition with the given type or nil, if it is not included.
func GetDeploymentCondition(conditions []appsv1.DeploymentCondition, conditionType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for _, cond := range conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}
