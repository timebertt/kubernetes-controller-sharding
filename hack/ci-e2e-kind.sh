#!/usr/bin/env bash

set -o nounset
set -o pipefail
set -o errexit

source "$(dirname "$0")/ci-common.sh"

# test setup
make kind-up
export KUBECONFIG=$PWD/hack/kind_kubeconfig.yaml

# export all container logs and events after test execution
trap '{
  export_artifacts
  make kind-down
}' EXIT

# deploy and test
make up SKAFFOLD_TAIL=false
make test-e2e GINKGO_FLAGS="--github-output"
