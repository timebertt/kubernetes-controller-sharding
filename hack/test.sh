#!/usr/bin/env bash

set -o nounset
set -o pipefail
set -o errexit

test_flags=
if [ -n "${CI:-}" ] ; then
  # Use Ginkgo timeout in CI to print everything that is buffered in GinkgoWriter.
  test_flags+=" --ginkgo.timeout=2m"
else
  # We don't want Ginkgo's timeout flag locally because it causes skipping the test cache.
  timeout_flag=-timeout=2m
fi

# shellcheck disable=SC2086
go test -race ${timeout_flag:-} "$@" $test_flags
