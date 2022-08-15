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

package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

const projectPrefix = "project-"

var (
	scheme = runtime.NewScheme()

	count      int
	namespaces []string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(webhostingv1alpha1.AddToScheme(scheme))
}

func main() {
	cmd := &cobra.Command{
		Use:  "samples-generator",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			return generateSamples(cmd.Context(), c)
		},
	}

	cmd.Flags().IntVarP(&count, "count", "c", count, "number of websites to generate per project")
	cmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", namespaces, "project namespaces to generate websites in")

	if err := cmd.ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Printf("Error generating samples: %v\n", err)
		os.Exit(1)
	}
}

func generateSamples(ctx context.Context, c client.Client) error {
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList); err != nil {
		return err
	}

	var themes []string
	for _, theme := range themeList.Items {
		themes = append(themes, theme.Name)
	}

	if len(themes) == 0 {
		return fmt.Errorf("no themes found, create them first")
	}

	namespaceList := &corev1.NamespaceList{}
	if err := c.List(ctx, namespaceList); err != nil {
		return err
	}

	var projects []string
	for _, namespace := range namespaceList.Items {
		if strings.HasPrefix(namespace.Name, projectPrefix) {
			projects = append(projects, namespace.Name)
		}
	}
	projects = filterProjects(projects)

	if len(projects) == 0 {
		return fmt.Errorf("no project namespaces found, create namespaces with prefix %q first", projectPrefix)
	}

	for _, project := range projects {
		websiteCount := rand.Intn(50) + 1
		if count > 0 {
			websiteCount = count
		}

		for i := 0; i < websiteCount; i++ {
			if err := c.Create(ctx, &webhostingv1alpha1.Website{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "sample-",
					Namespace:    project,
					Labels: map[string]string{
						"generated-by": "sample-generator",
					},
				},
				Spec: webhostingv1alpha1.WebsiteSpec{
					Theme: themes[rand.Intn(len(themes))],
				},
			}); err != nil {
				return err
			}
		}
		fmt.Printf("created %d Websites in project %q\n", websiteCount, project)
	}

	return nil
}

func filterProjects(in []string) []string {
	if len(namespaces) == 0 {
		return in
	}

	selected := sets.NewString(namespaces...)

	var out []string
	for _, project := range in {
		if !selected.Has(project) {
			continue
		}
		out = append(out, project)
	}
	return out
}
