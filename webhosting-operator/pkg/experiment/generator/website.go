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
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils"
)

// CreateWebsites creates n random websites.
func CreateWebsites(ctx context.Context, c client.Client, n int, opts ...GenerateOption) error {
	return NTimesConcurrently(n, 10, func() error {
		return CreateWebsite(ctx, c, opts...)
	})
}

// CreateWebsite creates a random website.
func CreateWebsite(ctx context.Context, c client.Client, opts ...GenerateOption) error {
	options := (&GenerateOptions{}).ApplyOptions(opts...)

	// pick random theme and project for a new website
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList, client.MatchingLabels(options.Labels)); err != nil {
		return err
	}
	namespaceList := &corev1.NamespaceList{}
	if err := c.List(ctx, namespaceList, client.MatchingLabels(options.Labels)); err != nil {
		return err
	}

	website := &webhostingv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "experiment-",
			Namespace:    utils.PickRandom(namespaceList.Items).Name,
		},
		Spec: webhostingv1alpha1.WebsiteSpec{
			Theme: utils.PickRandom(themeList.Items).Name,
		},
	}
	options.ApplyToObject(&website.ObjectMeta)

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

	patch := client.MergeFrom(website.DeepCopy())

	website.Spec.Theme = utils.PickRandom(themeList.Items).Name

	if err := c.Patch(ctx, website, patch); err != nil {
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
