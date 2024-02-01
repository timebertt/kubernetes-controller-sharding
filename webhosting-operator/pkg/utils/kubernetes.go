/*
Copyright 2023 Tim Ebert.

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

package utils

import (
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// RunningInCluster implements a heuristic for determining whether the process is running in a cluster or not.
func RunningInCluster() bool {
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		return true
	}

	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
}

// IsDeploymentReady returns true if the current generation has been observed by the deployment controller and has been
// fully rolled out.
func IsDeploymentReady(deployment *appsv1.Deployment) bool {
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return false
	}

	available := GetDeploymentCondition(deployment.Status.Conditions, appsv1.DeploymentAvailable)
	if available == nil || available.Status != corev1.ConditionTrue {
		return false
	}

	progressing := GetDeploymentCondition(deployment.Status.Conditions, appsv1.DeploymentProgressing)
	if progressing == nil || progressing.Status != corev1.ConditionTrue || progressing.Reason != "NewReplicaSetAvailable" {
		// only if Progressing is in status True with reason NewReplicaSetAvailable, the Deployment has been fully rolled out
		// note: old pods or excess pods (scale-down) might still be terminating, but there is no way to tell this from the
		// Deployment's status, see https://github.com/kubernetes/kubernetes/issues/110171
		return false
	}

	return true
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
