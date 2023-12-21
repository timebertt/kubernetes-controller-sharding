#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

# renovate: datasource=github-tags depName=prometheus-operator/kube-prometheus
KUBE_PROMETHEUS_VERSION=v0.13.0
echo "> Fetching kube-prometheus@$KUBE_PROMETHEUS_VERSION"

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

tarball="$tmp_dir/archive.tar.gz"
curl -sSLo "$tarball" https://github.com/prometheus-operator/kube-prometheus/archive/refs/tags/$KUBE_PROMETHEUS_VERSION.tar.gz

prometheus_operator_version=$(tar -xzf "$tarball" --wildcards "kube-prometheus-*/manifests/prometheusOperator-deployment.yaml" -O | grep app.kubernetes.io/version | head -1 | awk '{print $2}')
echo "Included prometheus-operator version: $prometheus_operator_version"

echo "> Updating CRDs"
pushd crds >/dev/null
rm *.yaml

tar -xzf "$tarball" --strip-components=3 --wildcards "kube-prometheus-*/manifests/setup/*.yaml"

# drop unneeded stuff
rm namespace.yaml

cat <<EOF > README.md
The CRDs in this directory were downloaded from
https://github.com/prometheus-operator/kube-prometheus/tree/$KUBE_PROMETHEUS_VERSION/manifests/setup.

Bump the version in [\`$(basename $0)\`](../$(basename $0)) and run the script to update the CRDs.
EOF

cat <<EOF > kustomization.yaml
# Code generated by $(basename $0), DO NOT EDIT.
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

commonLabels:
  app.kubernetes.io/name: prometheus-operator
  app.kubernetes.io/part-of: kube-prometheus
  app.kubernetes.io/version: $prometheus_operator_version

resources:
$(ls *.yaml | sed 's/^/- /')
EOF

popd >/dev/null

echo "> Updating kube-prometheus"
pushd kube-prometheus >/dev/null
rm *.yaml

tar -xzf "$tarball" --strip-components=2 --wildcards "kube-prometheus-*/manifests/*.yaml"

# drop unneeded stuff
rm -rf setup
rm alertmanager-*.yaml
# this will override metrics-server APIService (conflicts with gardener-resource-manager), drop it
rm prometheusAdapter-apiService.yaml
# PDB with minAvailable=1 for a single-replica StatefulSet blocks rolling node upgrades forever
rm prometheus-podDisruptionBudget.yaml

cat <<EOF > README.md
The manifests in this directory were downloaded from
https://github.com/prometheus-operator/kube-prometheus/tree/$KUBE_PROMETHEUS_VERSION/manifests.

Bump the version in [\`$(basename $0)\`](../$(basename $0)) and run the script to update the CRDs.
EOF

cat <<EOF > kustomization.yaml
# Code generated by $(basename $0), DO NOT EDIT.
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
$(ls *.yaml | sed 's/^/- /')
EOF

popd >/dev/null
