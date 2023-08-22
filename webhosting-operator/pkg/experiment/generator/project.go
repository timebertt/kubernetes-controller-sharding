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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateProjects creates n random project namespaces.
func CreateProjects(ctx context.Context, c client.Client, n int, opts ...GenerateOption) error {
	return NTimesConcurrently(n, 10, func() error {
		return CreateProject(ctx, c, opts...)
	})
}

// CreateProject creates a random project namespace.
func CreateProject(ctx context.Context, c client.Client, opts ...GenerateOption) error {
	options := (&GenerateOptions{}).ApplyOptions(opts...)

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "project-",
		},
	}
	options.ApplyToObject(&namespace.ObjectMeta)

	if err := c.Create(ctx, namespace); err != nil {
		return err
	}

	log.V(1).Info("Created project namespace", "namespaceName", namespace.Name)
	return nil
}

// CleanupProjects deletes all namespaces with the given labels.
func CleanupProjects(ctx context.Context, c client.Client, labels map[string]string) error {
	namespaceList := &corev1.NamespaceList{}
	if err := c.List(ctx, namespaceList, client.MatchingLabels(labels)); err != nil {
		return err
	}

	for _, namespace := range namespaceList.Items {
		if err := c.Delete(ctx, &namespace); err != nil {
			return err
		}
	}

	log.V(1).Info("Cleaned up all project namespaces")
	return nil
}
