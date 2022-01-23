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

package templates_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/templates"
	"github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/controllers/templates/internal"
)

var _ = Describe("nginx.conf", func() {
	const expectedConfig = `server {
    listen       80;
    listen  [::]:80;
    server_name  localhost;

    # rewrite /namespace/name to / for requests proxied from Ingress controller
    # this way, we don't need any Ingress controller specific configuration or objects
    rewrite ^/project-foo/homepage(.*)$ /$1 last;
    location / {
        root   /usr/share/nginx/html;
        index  index.html;
    }
}
`

	It("should successfully render nginx conf", func() {
		serverName, website, _ := internal.CreateExamples()
		Expect(RenderNginxConf(serverName, website)).To(Equal(expectedConfig))
	})
})
