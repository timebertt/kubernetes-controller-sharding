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

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# fetch code-generator module to execute the scripts from the modcache (we don't vendor here)
CODE_GENERATOR_DIR="$(go list -m -tags tools -f '{{ .Dir }}' k8s.io/code-generator)"
source "${CODE_GENERATOR_DIR}"/kube_codegen.sh

# setup virtual GOPATH
# k8s.io/code-generator does not work outside GOPATH, see https://github.com/kubernetes/kubernetes/issues/86753.
source "$SCRIPT_DIR"/vgopath-setup.sh

# We need to explicitly pass GO111MODULE=off to k8s.io/code-generator as it is significantly slower otherwise,
# see https://github.com/kubernetes/code-generator/issues/100.
export GO111MODULE=off

# sharder config API

sharder_config_group() {
  echo "Generating sharder config API group"

  kube::codegen::gen_helpers \
      --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
      "${SCRIPT_DIR}/../pkg/apis"
}

webhosting_config_group() {
  echo "Generating webhosting-operator config API group"

  kube::codegen::gen_helpers \
      --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
      "${SCRIPT_DIR}/../webhosting-operator/pkg/apis"
}

sharder_config_group
webhosting_config_group
