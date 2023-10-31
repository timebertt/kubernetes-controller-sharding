#!/usr/bin/env bash
# Copyright 2023 Tim Ebert.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# Friendly reminder if workspace location is not in $GOPATH
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
if [ "${SCRIPT_DIR}" != "$(realpath $GOPATH)/src/github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/hack" ]; then
  cat <<EOF
hack/update-codegen.sh does not work correctly if your workspace is outside GOPATH
because of a know bug in k8s.io/code-generator, see https://github.com/kubernetes/kubernetes/issues/86753.
Please move the workspace to $(realpath $GOPATH)/src/github.com/timebertt/kubernetes-controller-sharding.
EOF
  exit 1
fi

# fetch code-generator module to execute the scripts from the modcache (we don't vendor here)
CODE_GENERATOR_DIR="$(go list -m -tags tools -f '{{ .Dir }}' k8s.io/code-generator)"

rm -f ${GOPATH}/bin/*-gen

# config API

config_group() {
  echo "Generating config API group"

  bash "${CODE_GENERATOR_DIR}"/generate-internal-groups.sh \
      defaulter \
      github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/client/componentconfig \
      github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis \
      github.com/timebertt/kubernetes-controller-sharding/webhosting-operator/pkg/apis \
      "config:v1alpha1" \
      -h "${SCRIPT_DIR}/../../hack/boilerplate.go.txt"
}

config_group
