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
	"fmt"
	"net/http"

	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting/templates"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/controllers/webhosting/templates/internal"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serverName, website, theme := internal.CreateExamples()
		if err := templates.ExecuteIndexHTMLTemplate(w, serverName, website, theme); err != nil {
			http.Error(w, fmt.Sprintf("internal server error: %v", err), http.StatusInternalServerError)
		}
	})
	// nolint:gosec // this is just for testing
	if err := http.ListenAndServe(":9090", nil); err != nil {
		panic(err)
	}
}
