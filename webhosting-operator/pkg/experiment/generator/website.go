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
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils"
)

var (
	websiteTracker     WebsiteTracker
	websiteTrackerOnce sync.Once
)

type WebsiteTracker interface {
	RecordSpecChange(website *webhostingv1alpha1.Website)
}

// SetWebsiteTracker sets up this package to track website creations and spec updates with the given tracker.
func SetWebsiteTracker(tracker WebsiteTracker) {
	websiteTrackerOnce.Do(func() {
		websiteTracker = tracker
	})
}

// CreateWebsites creates n random websites.
func CreateWebsites(ctx context.Context, c client.Client, n int, opts ...GenerateOption) error {
	return NTimesConcurrently(n, 10, func() error {
		return RetryOnError(ctx, 5, func(ctx context.Context) error {
			return CreateWebsite(ctx, c, opts...)
		}, apierrors.IsAlreadyExists)
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
	if len(themeList.Items) == 0 {
		return fmt.Errorf("no themes found, cannot create website")
	}

	namespaceList := &corev1.NamespaceList{}
	if err := c.List(ctx, namespaceList, client.MatchingLabels(options.Labels)); err != nil {
		return err
	}
	if len(namespaceList.Items) == 0 {
		return fmt.Errorf("no namespaces found, cannot create website")
	}

	website := &webhostingv1alpha1.Website{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "experiment-" + utils.RandomName(8),
			Namespace: utils.PickRandom(namespaceList.Items).Name,
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
	if websiteTracker != nil {
		websiteTracker.RecordSpecChange(website)
	}

	return nil
}

// MutateWebsite mutates the given website using the given client and labels.
func MutateWebsite(ctx context.Context, c client.Client, website *webhostingv1alpha1.Website, labels map[string]string) error {
	// pick new random theme for the website
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList, client.MatchingLabels(labels)); err != nil {
		return err
	}
	if len(themeList.Items) == 0 {
		log.V(1).Info("No themes found, skipping mutation")
		return nil
	}

	patch := client.MergeFrom(website.DeepCopy())

	website.Spec.Theme = utils.PickRandom(themeList.Items).Name

	if err := c.Patch(ctx, website, patch); err != nil {
		return err
	}

	log.V(1).Info("Mutated website", "website", client.ObjectKeyFromObject(website))
	if websiteTracker != nil {
		websiteTracker.RecordSpecChange(website)
	}

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
		log.V(1).Info("No websites found, skipping deletion")
		return nil
	}

	website := utils.PickRandom(websiteList.Items)
	if err := c.Delete(ctx, &website); err != nil {
		return err
	}

	log.V(1).Info("Deleted website", "website", client.ObjectKeyFromObject(&website))
	return nil
}
