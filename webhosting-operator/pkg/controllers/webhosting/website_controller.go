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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/config/v1alpha1"
	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting/templates"
)

// WebsiteReconciler reconciles a Website object.
type WebsiteReconciler struct {
	Client        client.Client
	ShardedClient client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	logger        logr.Logger

	Config *configv1alpha1.WebhostingOperatorConfig
}

//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=websites/finalizers,verbs=update
//+kubebuilder:rbac:groups=webhosting.timebertt.dev,resources=themes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// RBAC required for sharding
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;update;patch;delete

// Reconcile reconciles a Website object.
func (r *WebsiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling website")

	website := &webhostingv1alpha1.Website{}
	if err := r.ShardedClient.Get(ctx, req.NamespacedName, website); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Object is gone, stop reconciling")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	before := website.DeepCopy()
	// always update the status with the latest observed generation
	website.Status.ObservedGeneration = website.Generation

	reconcileErr := r.reconcileWebsite(ctx, log, website)
	if reconcileErr != nil {
		website.Status.Phase = webhostingv1alpha1.PhaseError
	}

	// update the status if needed
	if !apiequality.Semantic.DeepEqual(before.Status, website.Status) {
		website.Status.LastTransitionTime = ptr.To(metav1.NowMicro())

		if err := r.ShardedClient.Status().Update(ctx, website); err != nil {
			// unable to update status, requeue with backoff
			if reconcileErr != nil {
				// if a reconcile error happened as well, prefer this error
				return reconcile.Result{}, reconcileErr
			}

			return reconcile.Result{}, fmt.Errorf("failed updating Website status: %w", err)
		}
	}

	return reconcile.Result{}, reconcileErr
}

func (r *WebsiteReconciler) reconcileWebsite(ctx context.Context, log logr.Logger, website *webhostingv1alpha1.Website) error {
	if website.DeletionTimestamp != nil {
		// Nothing to do on deletion, all owned objects are cleaned up by the garbage collector.
		// Set the website's status to terminating and be done with it.
		// Note: we will only execute this part of the code if the website is deleted with foreground deletion policy,
		// otherwise it will be gone immediately.
		website.Status.Phase = webhostingv1alpha1.PhaseTerminating

		return nil
	}

	if website.Spec.Theme == "" {
		err := r.recordError(website, "ThemeUnspecified", "Website doesn't specify a Theme")

		// Only requeue with backoff if we fail to update the status. We can't do much till the spec changes, so rather wait
		// for the next update event.
		// Log the error and forget the object for now.
		log.Error(err, "Unable to reconcile Website")
		return nil
	}

	// retrieve theme
	theme := &webhostingv1alpha1.Theme{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: website.Spec.Theme}, theme); err != nil {
		if apierrors.IsNotFound(err) {
			return r.recordError(website, "ThemeNotFound", "Theme %s not found", website.Spec.Theme)
		}
		return r.recordError(website, "ReconcilerError", "Error getting Theme %s: %v", website.Spec.Theme, err)
	}

	serverName := calculateServerName(website)
	log = log.WithValues("theme", website.Spec.Theme, "serverName", serverName)

	// create downstream objects
	configMap, err := r.ConfigMapForWebsite(serverName, website, theme)
	if err != nil {
		return r.recordError(website, "ReconcilerError", "Error computing ConfigMap: %v", err)
	}
	if err := r.ShardedClient.Patch(ctx, configMap, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return r.recordError(website, "ReconcilerError", "Error applying ConfigMap: %v", err)
	}

	service, err := r.ServiceForWebsite(serverName, website)
	if err != nil {
		return r.recordError(website, "ReconcilerError", "Error computing Service: %v", err)
	}
	if err := r.ShardedClient.Patch(ctx, service, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return r.recordError(website, "ReconcilerError", "Error applying Service: %v", err)
	}

	ingress, err := r.IngressForWebsite(serverName, website)
	if err != nil {
		return r.recordError(website, "ReconcilerError", "Error computing Ingress: %v", err)
	}
	if err := r.ShardedClient.Patch(ctx, ingress, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return r.recordError(website, "ReconcilerError", "Error applying Ingress: %v", err)
	}

	deployment, err := r.DeploymentForWebsite(serverName, website, configMap)
	if err != nil {
		return r.recordError(website, "ReconcilerError", "Error computing Deployment: %v", err)
	}
	if err := r.ShardedClient.Patch(ctx, deployment, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return r.recordError(website, "ReconcilerError", "Error applying Deployment: %v", err)
	}

	// update status
	newPhase := webhostingv1alpha1.PhasePending
	if IsDeploymentAvailable(deployment) {
		newPhase = webhostingv1alpha1.PhaseReady
	}
	website.Status.Phase = newPhase

	return nil
}

func (r *WebsiteReconciler) recordError(website *webhostingv1alpha1.Website, reason, messageFmt string, args ...interface{}) error {
	r.Recorder.Eventf(website, corev1.EventTypeWarning, reason, messageFmt, args...)

	// this error can be returned by the reconciler to retry with backoff
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

	if isGeneratedByExperiment(website) {
		// don't actually expose website ingresses in load tests
		// use fake ingress class to prevent overloading ingress controller (this is not what we want to load test)
		ingress.Spec.IngressClassName = pointer.String("fake")
	}

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
			ingress.Spec.TLS[i] = *tls.DeepCopy()
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

	if isGeneratedByExperiment(website) {
		// don't actually run website pods in load tests
		// otherwise, we would need an immense amount of compute power for running dummy websites
		deployment.Spec.Replicas = pointer.Int32(0)
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
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.ShardedClient == nil {
		r.ShardedClient = mgr.GetShardedClient()
	}
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

	workers := 15
	if !mgr.IsSharded() {
		// When comparing singleton vs sharded setups, the singleton will fail to verify the SLOs because it has too few
		// website workers. Increase the worker count to allow comparing the setups.
		workers = 50
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&webhostingv1alpha1.Website{}, builder.Sharded{}, builder.WithPredicates(
			// trigger on spec change and annotation changes (manual trigger for testing purposes)
			predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{})),
		).
		// watch deployments in order to update phase on relevant changes
		// watch deployments for relevant changes to reconcile them back if changed
		Owns(&appsv1.Deployment{}, builder.Sharded{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			DeploymentAvailabilityChanged,
		))).
		// watch owned objects for relevant changes to reconcile them back if changed
		Owns(&corev1.ConfigMap{}, builder.Sharded{}, builder.WithPredicates(ConfigMapDataChanged)).
		Owns(&corev1.Service{}, builder.Sharded{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&networkingv1.Ingress{}, builder.Sharded{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// watch themes to roll out theme changes to all referencing websites
		Watches(
			&webhostingv1alpha1.Theme{},
			handler.EnqueueRequestsFromMapFunc(r.MapThemeToWebsites),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: workers,
		}).
		Build(SilenceConflicts(r))
	if err != nil {
		return err
	}

	r.logger = c.GetLogger()

	return nil
}

// MapThemeToWebsites maps a theme to all websites that use it.
func (r *WebsiteReconciler) MapThemeToWebsites(ctx context.Context, theme client.Object) []reconcile.Request {
	websiteList := &webhostingv1alpha1.WebsiteList{}
	if err := r.ShardedClient.List(ctx, websiteList, client.MatchingFields{websiteThemeField: theme.GetName()}); err != nil {
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

// DeploymentAvailabilityChanged is a predicate for filtering relevant Deployment events.
var DeploymentAvailabilityChanged = predicate.Funcs{
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

		return IsDeploymentAvailable(oldDeployment) != IsDeploymentAvailable(newDeployment)
	},
}

// ConfigMapDataChanged is a predicate for filtering relevant ConfigMap events.
// Similar to predicate.GenerationChangedPredicate (ConfigMaps don't have a generation).
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

// IsDeploymentAvailable returns true if the current generation has been observed by the deployment controller and the
// Available condition is True.
func IsDeploymentAvailable(deployment *appsv1.Deployment) bool {
	available := GetDeploymentCondition(deployment.Status.Conditions, appsv1.DeploymentAvailable)
	return deployment.Status.ObservedGeneration == deployment.Generation && available != nil && available.Status == corev1.ConditionTrue
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

func isGeneratedByExperiment(obj client.Object) bool {
	return obj.GetLabels()["generated-by"] == "experiment"
}
