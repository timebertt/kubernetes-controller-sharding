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
	"html/template"
	"io"
	"strings"

	webhostingv1alpha1 "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/apis/webhosting/v1alpha1"
)

var (
	//go:embed index.tmpl
	indexTplFS embed.FS
	// parsed template
	indexTpl *template.Template
)

func init() {
	var err error
	indexTpl, err = template.New("index.tmpl").ParseFS(indexTplFS, "*.tmpl")
	if err != nil {
		panic(fmt.Errorf("error parsing template: %w", err))
	}
}

// RenderIndexHTML renders the index template and returns it as a string.
func RenderIndexHTML(serverName string, website *webhostingv1alpha1.Website, theme *webhostingv1alpha1.Theme) (string, error) {
	out := &strings.Builder{}
	err := ExecuteIndexHTMLTemplate(out, serverName, website, theme)
	return out.String(), err
}

// ExecuteIndexHTMLTemplate renders the index template and writes it to the given io.Writer.
func ExecuteIndexHTMLTemplate(w io.Writer, serverName string, website *webhostingv1alpha1.Website, theme *webhostingv1alpha1.Theme) error {
	if err := indexTpl.Execute(w, struct {
		ServerName string
		Website    *webhostingv1alpha1.Website
		Theme      *webhostingv1alpha1.Theme
	}{serverName, website, theme}); err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}
	return nil
}
