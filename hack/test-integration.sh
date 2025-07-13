#!/usr/bin/env bash

set -o nounset
set -o pipefail
set -o errexit

ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.32"}

# shellcheck disable=SC1090
# --use-env allows overwriting the envtest tools path via the KUBEBUILDER_ASSETS env var
source <(setup-envtest use --use-env -p env "${ENVTEST_K8S_VERSION}")
echo "Using envtest binaries installed at '$KUBEBUILDER_ASSETS'"

source "$(dirname "$0")/test-integration.env"

test_flags=
if [ -n "${CI:-}" ] ; then
  # Use Ginkgo timeout in CI to print everything that is buffered in GinkgoWriter.
  test_flags+=" --ginkgo.timeout=5m"
else
  # We don't want Ginkgo's timeout flag locally because it causes skipping the test cache.
  timeout_flag=-timeout=5m
fi

# shellcheck disable=SC2086
go test ${timeout_flag:-} "$@" $test_flags
