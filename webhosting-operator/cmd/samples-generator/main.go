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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/webhosting/v1alpha1"
)

const projectPrefix = "project-"

var ctx = context.TODO()

func main() {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(webhostingv1alpha1.AddToScheme(scheme))

	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	utilruntime.Must(err)

	utilruntime.Must(generateSamples(c))
}

func generateSamples(c client.Client) error {
	themeList := &webhostingv1alpha1.ThemeList{}
	if err := c.List(ctx, themeList); err != nil {
		return err
	}

	var themes []string
	for _, theme := range themeList.Items {
		themes = append(themes, theme.Name)
	}

	if len(themes) == 0 {
		fmt.Printf("No themes found, create them first!\n")
		os.Exit(1)
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

	if len(projects) == 0 {
		fmt.Printf("No project namespaces found, create namespaces with prefix %q first!\n", projectPrefix)
		os.Exit(1)
	}

	for _, project := range projects {
		websiteCount := rand.Intn(50) + 1
		for i := 0; i < websiteCount; i++ {
			if err := c.Create(ctx, &webhostingv1alpha1.Website{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "sample-",
					Namespace:    project,
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
