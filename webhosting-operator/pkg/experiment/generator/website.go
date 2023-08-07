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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/experiment/utils"
)

// EnsureWebsites ensures there are exactly n websites with the given labels.
// It keeps existing websites to speed up experiment preparation.
func EnsureWebsites(ctx context.Context, c client.Client, labels map[string]string, n int) error {
	// delete excess websites
	websiteList := &webhostingv1alpha1.WebsiteList{}
	if err := c.List(ctx, websiteList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	for _, theme := range utils.PickNRandom(websiteList.Items, len(websiteList.Items)-n) {
		if err := c.Delete(ctx, &theme); err != nil {
			return err
		}
	}

	// create missing websites
	for i := 0; i < n-len(websiteList.Items); i++ {
		if err := CreateWebsite(ctx, c, labels); err != nil {
			return err
		}
	}

	return nil
}

// CreateWebsite creates a random website using the given client and labels.
func CreateWebsite(ctx context.Context, c client.Client, labels map[string]string) error {
	// pick random theme and project for a new website
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList, client.MatchingLabels(labels)); err != nil {
		return err
	}
	namespaceList := &corev1.NamespaceList{}
	if err := c.List(ctx, namespaceList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	website := &webhostingv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "experiment-",
			Namespace:    utils.PickRandom(namespaceList.Items).Name,
			Labels:       utils.CopyMap(labels),
		},
		Spec: webhostingv1alpha1.WebsiteSpec{
			Theme: utils.PickRandom(themeList.Items).Name,
		},
	}

	if err := c.Create(ctx, website); err != nil {
		return err
	}

	log.V(1).Info("Created website", "website", client.ObjectKeyFromObject(website))
	return nil
}

// MutateWebsite mutates the given website using the given client and labels.
func MutateWebsite(ctx context.Context, c client.Client, website *webhostingv1alpha1.Website, labels map[string]string) error {
	// pick new random theme for the website
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	website.Spec.Theme = utils.PickRandom(themeList.Items).Name

	if err := c.Update(ctx, website); err != nil {
		return err
	}

	log.V(1).Info("Mutated website", "website", client.ObjectKeyFromObject(website))
	return nil
}

// ReconcileWebsite triggers reconciliation of the given website by updating an annotation.
func ReconcileWebsite(ctx context.Context, c client.Client, website *webhostingv1alpha1.Website) error {
	patch := client.MergeFrom(website.DeepCopy())
	metav1.SetMetaDataAnnotation(&website.ObjectMeta, "experiment-reconcile", time.Now().UTC().Format(time.RFC3339))

	if err := c.Patch(ctx, website, patch); err != nil {
		return err
	}

	log.V(1).Info("Triggered reconciliation of website", "website", client.ObjectKeyFromObject(website))
	return nil
}

// DeleteWebsite deletes a random existing website using the given client and labels.
func DeleteWebsite(ctx context.Context, c client.Client, labels map[string]string) error {
	websiteList := &webhostingv1alpha1.WebsiteList{}
	if err := c.List(ctx, websiteList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	if len(websiteList.Items) == 0 {
		log.V(1).Info("No websites created yet, skipping mutation")
		return nil
	}

	website := utils.PickRandom(websiteList.Items)
	if err := c.Delete(ctx, &website); err != nil {
		return err
	}

	log.V(1).Info("Deleted website", "website", client.ObjectKeyFromObject(&website))
	return nil
}
