/*
Copyright 2025 Tim Ebert.

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

package test

import (
	"os"
	"path/filepath"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// RepoRoot returns a relative path from the current working directory to the repository root. For use in tests only.
func RepoRoot() string {
	relativePath := "."

	dir, err := os.Getwd()
	utilruntime.Must(err)

	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			break
		} else if !os.IsNotExist(err) {
			panic(err)
		}

		// navigate one directory up, build up relative path
		dir = filepath.Dir(dir)
		relativePath = filepath.Join(relativePath, "..")
	}

	return filepath.Clean(relativePath)
}

// PathShardingCRDs returns the path to the directory containing the sharding CRDs. For use in tests only.
func PathShardingCRDs() string {
	return filepath.Join(RepoRoot(), "config", "crds")
}
