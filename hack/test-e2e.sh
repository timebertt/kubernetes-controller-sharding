#!/usr/bin/env bash

set -o nounset
set -o pipefail
set -o errexit

source "$(dirname "$0")/test-e2e.env"

ginkgo run --timeout=1h --poll-progress-after=60s --poll-progress-interval=30s --randomize-all --randomize-suites --keep-going --vv "$@"
