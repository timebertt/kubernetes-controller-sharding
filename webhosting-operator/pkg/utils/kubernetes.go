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
)

// RunningInCluster implements a heuristic for determining whether the process is running in a cluster or not.
func RunningInCluster() bool {
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err == nil {
		return true
	}

	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
}
