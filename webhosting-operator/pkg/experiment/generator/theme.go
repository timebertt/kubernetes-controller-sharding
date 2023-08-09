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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/utils"
)

var (
	themeColors = []string{"aqua", "black", "blue", "fuchsia", "gray", "green", "lime", "maroon", "navy", "olive", "orange", "purple", "red", "silver", "teal", "white", "yellow"}
	themeFonts  = []string{"Arial", "Verdana", "Tahoma", "Trebuchet MS", "Times New Roman", "Georgia", "Garamond", "Courier New", "Brush Script MT"}
)

// EnsureThemes ensures there are exactly n themes with the given labels.
// It keeps existing themes to speed up experiment preparation.
func EnsureThemes(ctx context.Context, c client.Client, labels map[string]string, n int) error {
	// delete excess themes
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	for _, theme := range utils.PickNRandom(themeList.Items, len(themeList.Items)-n) {
		if err := c.Delete(ctx, &theme); err != nil {
			return err
		}
	}

	// create missing themes
	for i := 0; i < n-len(themeList.Items); i++ {
		if err := CreateTheme(ctx, c, labels); err != nil {
			return err
		}
	}

	return nil
}

// CreateTheme creates a random theme using the given client and labels.
func CreateTheme(ctx context.Context, c client.Client, labels map[string]string) error {
	theme := &webhostingv1alpha1.Theme{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "theme-",
			Labels:       utils.CopyMap(labels),
		},
		Spec: webhostingv1alpha1.ThemeSpec{
			Color:      utils.PickRandom(themeColors),
			FontFamily: utils.PickRandom(themeFonts),
		},
	}

	if err := c.Create(ctx, theme); err != nil {
		return err
	}

	log.V(1).Info("Created theme", "themeName", theme.Name)
	return nil
}

// MutateTheme mutates the given theme using the given client and labels.
func MutateTheme(ctx context.Context, c client.Client, theme *webhostingv1alpha1.Theme) error {
	patch := client.MergeFrom(theme.DeepCopy())

	theme.Spec.Color = utils.PickRandom(themeColors)
	theme.Spec.FontFamily = utils.PickRandom(themeFonts)

	if err := c.Patch(ctx, theme, patch); err != nil {
		return err
	}

	log.V(1).Info("Mutated theme", "themeName", theme.Name)
	return nil
}

// MutateRandomTheme mutates a random existing theme using the given client and labels.
func MutateRandomTheme(ctx context.Context, c client.Client, labels map[string]string) error {
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	if len(themeList.Items) == 0 {
		log.V(1).Info("No themes created yet, skipping mutation")
		return nil
	}

	theme := utils.PickRandom(themeList.Items)
	return MutateTheme(ctx, c, &theme)
}

// CleanupThemes deletes all themes with the given labels.
func CleanupThemes(ctx context.Context, c client.Client, labels map[string]string) error {
	if err := c.DeleteAllOf(ctx, &webhostingv1alpha1.Theme{}, client.MatchingLabels(labels)); err != nil {
		return err
	}

	log.V(1).Info("Cleaned up all project themes")
	return nil
}
