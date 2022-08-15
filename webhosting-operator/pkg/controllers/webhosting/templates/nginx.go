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

package templates

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis/webhosting/v1alpha1"
)

var (
	//go:embed nginx.conf.tmpl
	nginxTplFS embed.FS
	// parsed template
	nginxTpl *template.Template
)

func init() {
	var err error
	nginxTpl, err = template.New("nginx.conf.tmpl").ParseFS(nginxTplFS, "*.tmpl")
	if err != nil {
		panic(fmt.Errorf("error parsing template: %w", err))
	}
}

// RenderNginxConf renders the nginx config template and returns it as a string.
func RenderNginxConf(serverName string, website *webhostingv1alpha1.Website) (string, error) {
	out := &strings.Builder{}
	if err := nginxTpl.Execute(out, struct {
		ServerName string
		Website    *webhostingv1alpha1.Website
	}{serverName, website}); err != nil {
		return "", fmt.Errorf("error rendering template: %w", err)
	}
	return out.String(), nil
}
