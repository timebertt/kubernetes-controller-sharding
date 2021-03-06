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
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/config/v1alpha1"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/webhosting/templates"
)

// WebsiteReconciler reconciles a Website object.
type WebsiteReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	logger   logr.Logger

	Config *configv1alpha1.ControllerManagerConfig
}

//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=list;watch;create;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=list;watch;create;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;watch;create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch;create;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// RBAC required for sharding
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;update;patch;delete

// InjectClient injects a client that has access to both the sharded cache and un-sharded cache.
func (r *WebsiteReconciler) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}

// Reconcile reconciles a Website object.
func (r *WebsiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling website")

	website := &webhostingv1alpha1.Website{}
	if err := r.Client.Get(ctx, req.NamespacedName, website); err != nil {
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
		return ctrl.Result{}, r.Client.Status().Update(ctx, website)
	}

	// retrieve theme
	theme := &webhostingv1alpha1.Theme{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: website.Spec.Theme}, theme); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ThemeNotFound", "Theme %s not found", website.Spec.Theme)
		}
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error getting Theme %s: %v", website.Spec.Theme, err)
	}

	serverName := calculateServerName(website)
	log = log.WithValues("theme", website.Spec.Theme, "serverName", serverName)

	// get current deployment status
	currentDeployment := &appsv1.Deployment{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: website.Namespace, Name: serverName}, currentDeployment); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error getting Deployment: %v", err)
	}

	// create downstream objects
	configMap, err := r.ConfigMapForWebsite(serverName, website, theme)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing ConfigMap: %v", err)
	}
	if err := r.Client.Patch(ctx, configMap, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying ConfigMap: %v", err)
	}

	service, err := r.ServiceForWebsite(serverName, website)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing Service: %v", err)
	}
	if err := r.Client.Patch(ctx, service, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying Service: %v", err)
	}

	ingress, err := r.IngressForWebsite(serverName, website)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing Ingress: %v", err)
	}
	if err := r.Client.Patch(ctx, ingress, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying Ingress: %v", err)
	}

	deployment, err := r.DeploymentForWebsite(serverName, website, configMap)
	if err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error computing Deployment: %v", err)
	}
	if err := r.Client.Patch(ctx, deployment, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, r.recordErrorAndUpdateStatus(ctx, website, "ReconcilerError", "Error applying Deployment: %v", err)
	}

	// update status
	newPhase := webhostingv1alpha1.PhasePending
	if cond := GetDeploymentCondition(currentDeployment.Status.Conditions, appsv1.DeploymentAvailable); cond != nil && cond.Status == corev1.ConditionTrue {
		newPhase = webhostingv1alpha1.PhaseReady
	}
	website.Status.Phase = newPhase

	return ctrl.Result{}, r.Client.Status().Update(ctx, website)
}

func (r *WebsiteReconciler) recordErrorAndUpdateStatus(ctx context.Context, website *webhostingv1alpha1.Website, reason, messageFmt string, args ...interface{}) error {
	r.Recorder.Eventf(website, corev1.EventTypeWarning, reason, messageFmt, args...)

	website.Status.Phase = webhostingv1alpha1.PhaseError
	if err := r.Client.Status().Update(ctx, website); err != nil {
		// unable to update status, requeue with backoff
		return err
	}
	// return error to retry with backoff
	return fmt.Errorf(messageFmt, args...)
}

const (
	keyIndexHTML = "index.html"
	keyNginxConf = "nginx.conf"
	portNameHTTP = "http"
)

// ConfigMapForWebsite creates a ConfigMap object to be applied for the given website.
func (r *WebsiteReconciler) ConfigMapForWebsite(serverName string, website *webhostingv1alpha1.Website, theme *webhostingv1alpha1.Theme) (*corev1.ConfigMap, error) {
	indexHTML, err := templates.RenderIndexHTML(serverName, website, theme)
	if err != nil {
		return nil, err
	}
	nginxConf, err := templates.RenderNginxConf(serverName, website)
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName,
			Namespace: website.Namespace,
			Labels:    getLabelsForServer(website.Name, serverName),
		},
		Data: map[string]string{
			keyIndexHTML: indexHTML,
			keyNginxConf: nginxConf,
		},
	}

	return configMap, ctrl.SetControllerReference(website, configMap, r.Scheme)
}

// ServiceForWebsite creates a Service object to be applied for the given website.
func (r *WebsiteReconciler) ServiceForWebsite(serverName string, website *webhostingv1alpha1.Website) (*corev1.Service, error) {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName,
			Namespace: website.Namespace,
			Labels:    getLabelsForServer(website.Name, serverName),
		},
		Spec: corev1.ServiceSpec{
			Selector: getLabelsForServer(website.Name, serverName),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{{
				Name:       portNameHTTP,
				Port:       8080,
				TargetPort: intstr.FromString(portNameHTTP),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}

	return service, ctrl.SetControllerReference(website, service, r.Scheme)
}

// IngressForWebsite creates a Ingress object to be applied for the given website.
func (r *WebsiteReconciler) IngressForWebsite(serverName string, website *webhostingv1alpha1.Website) (*networkingv1.Ingress, error) {
	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: networkingv1.SchemeGroupVersion.String(),
			Kind:       "Ingress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName,
			Namespace: website.Namespace,
			Labels:    getLabelsForServer(website.Name, serverName),
		},
	}

	// base ingress rule value
	pathType := networkingv1.PathTypePrefix
	ingressRuleValue := networkingv1.IngressRuleValue{
		HTTP: &networkingv1.HTTPIngressRuleValue{
			Paths: []networkingv1.HTTPIngressPath{{
				Path:     fmt.Sprintf("/%s/%s", website.Namespace, website.Name),
				PathType: &pathType,
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: serverName,
						Port: networkingv1.ServiceBackendPort{
							Name: portNameHTTP,
						},
					},
				},
			}},
		},
	}

	// add default rule without hosts
	ingress.Spec.Rules = []networkingv1.IngressRule{{
		IngressRuleValue: ingressRuleValue,
	}}

	applyIngressConfigToIngress(r.Config.Ingress, ingress)
	return ingress, ctrl.SetControllerReference(website, ingress, r.Scheme)
}

func applyIngressConfigToIngress(config *configv1alpha1.IngressConfiguration, ingress *networkingv1.Ingress) {
	if config == nil {
		// nothing to apply, go with defaults
		return
	}

	// apply annotations
	for key, value := range config.Annotations {
		metav1.SetMetaDataAnnotation(&ingress.ObjectMeta, key, value)
	}

	// apply hosts
	if len(config.Hosts) > 0 {
		// use default rule for each host in config
		ingressRuleValue := ingress.Spec.Rules[0].IngressRuleValue

		ingress.Spec.Rules = make([]networkingv1.IngressRule, len(config.Hosts))
		for i, host := range config.Hosts {
			ingress.Spec.Rules[i] = networkingv1.IngressRule{
				Host:             host,
				IngressRuleValue: ingressRuleValue,
			}
		}
	}

	// apply TLS config
	if len(config.TLS) > 0 {
		ingress.Spec.TLS = make([]networkingv1.IngressTLS, len(config.TLS))
		for i, tls := range config.TLS {
			tls = *tls.DeepCopy()
			// tls secret not given, generate website specific secret name
			if tls.SecretName == "" {
				tls.SecretName = ingress.Name + "-tls"
			}
			ingress.Spec.TLS[i] = tls
		}
	}
}

// DeploymentForWebsite creates a Deployment object to be applied for the given website.
func (r *WebsiteReconciler) DeploymentForWebsite(serverName string, website *webhostingv1alpha1.Website, configMap *corev1.ConfigMap) (*appsv1.Deployment, error) {
	configMapChecksum, err := calculateConfigMapChecksum(configMap)
	if err != nil {
		return nil, fmt.Errorf("error calculating checksum of ConfigMap: %w", err)
	}

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName,
			Namespace: website.Namespace,
			Labels:    getLabelsForServer(website.Name, serverName),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: getLabelsForServer(website.Name, serverName),
			},
			Replicas:             pointer.Int32(1),
			RevisionHistoryLimit: pointer.Int32(2),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: getLabelsForServer(website.Name, serverName),
					Annotations: map[string]string{
						"checksum/configmap": configMapChecksum,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.21-alpine",
						Ports: []corev1.ContainerPort{{
							Name:          portNameHTTP,
							ContainerPort: 80,
							Protocol:      corev1.ProtocolTCP,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "website-data",
							ReadOnly:  true,
							MountPath: "/usr/share/nginx/html",
						}, {
							Name:      "website-config",
							ReadOnly:  true,
							MountPath: "/etc/nginx/conf.d",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "website-data",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: configMap.Name},
								Items: []corev1.KeyToPath{{
									Key:  keyIndexHTML,
									Path: "index.html",
								}},
							},
						},
					}, {
						Name: "website-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: configMap.Name},
								Items: []corev1.KeyToPath{{
									Key:  keyNginxConf,
									Path: "nginx.conf",
								}},
							},
						},
					}},
				},
			},
		},
	}

	return deployment, ctrl.SetControllerReference(website, deployment, r.Scheme)
}

func getLabelsForServer(name, serverName string) map[string]string {
	return map[string]string{
		"app":        "website",
		"website":    name,
		"server":     serverName,
		"managed-by": "webhosting-operator",
	}
}

func calculateServerName(website *webhostingv1alpha1.Website) string {
	// Customers might delete the website and create a new one with the same name.
	// To avoid clashes in that case, we need to include the website's UID in the name of owned objects.
	// Take a sha256 sum and include the first 6 hex characters.
	checksum := sha256.Sum256([]byte(website.UID))
	return website.Name + "-" + hex.EncodeToString(checksum[:])[:6]
}

// calculateConfigMapChecksum calculates a checksum of the given ConfigMap's data. It is supposed to be added to the
// pod template to trigger rolling updates on ConfigMap changes. This is to force nginx to reload changed config and
// content.
func calculateConfigMapChecksum(configMap *corev1.ConfigMap) (string, error) {
	dataBytes, err := json.Marshal(configMap.Data)
	if err != nil {
		return "", err
	}

	checksum := sha256.Sum256(dataBytes)
	return hex.EncodeToString(checksum[:]), nil
}

const websiteThemeField = "spec.theme"

// SetupWithManager sets up the controller with the Manager.
func (r *WebsiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Scheme == nil {
		r.Scheme = mgr.GetScheme()
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("website-controller")
	}

	if err := mgr.GetShardedCache().IndexField(context.TODO(), &webhostingv1alpha1.Website{}, websiteThemeField, func(obj client.Object) []string {
		return []string{obj.(*webhostingv1alpha1.Website).Spec.Theme}
	}); err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&webhostingv1alpha1.Website{}, builder.Sharded{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// watch deployments in order to update phase on relevant changes
		Owns(&appsv1.Deployment{}, builder.Sharded{}, builder.WithPredicates(DeploymentReadinessChanged)).
		// watch owned objects for relevant changes to reconcile them back if changed
		Owns(&corev1.ConfigMap{}, builder.Sharded{}, builder.WithPredicates(ConfigMapDataChanged)).
		Owns(&corev1.Service{}, builder.Sharded{}, builder.WithPredicates(ServiceSpecChanged)).
		Owns(&networkingv1.Ingress{}, builder.Sharded{}, builder.WithPredicates(IngressSpecChanged)).
		// watch themes to roll out theme changes to all referencing websites
		Watches(
			&source.Kind{Type: &webhostingv1alpha1.Theme{}},
			handler.EnqueueRequestsFromMapFunc(r.MapThemeToWebsites),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
			RecoverPanic:            true,
		}).
		Build(r)
	if err != nil {
		return err
	}

	r.logger = c.GetLogger()

	return nil
}

// MapThemeToWebsites maps a theme to all websites that use it.
func (r *WebsiteReconciler) MapThemeToWebsites(theme client.Object) []reconcile.Request {
	websiteList := &webhostingv1alpha1.WebsiteList{}
	if err := r.Client.List(context.TODO(), websiteList, client.MatchingFields{websiteThemeField: theme.GetName()}); err != nil {
		r.logger.Error(err, "failed to list websites belonging to theme", "theme", client.ObjectKeyFromObject(theme))
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

// DeploymentReadinessChanged is a predicate for filtering relevant Deployment events.
var DeploymentReadinessChanged = predicate.Funcs{
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

		if !apiequality.Semantic.DeepEqual(oldDeployment.Status.ReadyReplicas, newDeployment.Status.ReadyReplicas) {
			return true
		}

		oldAvailable := GetDeploymentCondition(oldDeployment.Status.Conditions, appsv1.DeploymentAvailable)
		newAvailable := GetDeploymentCondition(newDeployment.Status.Conditions, appsv1.DeploymentAvailable)
		return !apiequality.Semantic.DeepEqual(oldAvailable, newAvailable)
	},
}

// ConfigMapDataChanged is a predicate for filtering relevant ConfigMap events.
var ConfigMapDataChanged = predicate.Funcs{
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

// IngressSpecChanged is a predicate for filtering relevant Ingress events.
var IngressSpecChanged = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}

		oldIngress, ok := e.ObjectOld.(*networkingv1.Ingress)
		if !ok {
			return false
		}
		newIngress, ok := e.ObjectNew.(*networkingv1.Ingress)
		if !ok {
			return false
		}
		return !apiequality.Semantic.DeepEqual(oldIngress.Spec, newIngress.Spec)
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
