#!/usr/bin/env bash

# This script generates comma-separated tags and labels for image builds (similar to docker/metadata-action).
# It writes its output to the github output variable file or env file. It can only be used in github actions.

set -x
set -o errexit
set -o pipefail
set -o nounset

build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
# revision is the commit sha
revision=$GITHUB_SHA
# short_ref is the branch name, or the semver git tag
short_ref=$GITHUB_REF_NAME
# version is the semantic version
version=v0.1.0-dev # used if no semver tag has been pushed yet
major_version="0"
minor_version="1"

tags=( "sha-$( echo "$revision" | head -c7 )" )
if [[ $short_ref = main ]] ; then
  tags+=( latest )
fi

if [[ $GITHUB_REF = refs/tags/* && $short_ref =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)([-].*)?([+].*)?$ ]] ; then
  # triggered for a semver tag, add it as image tag
  tags+=( "$short_ref" )

  # extract version information
  version=$short_ref

  major_version=${BASH_REMATCH[1]}
  minor_version=${BASH_REMATCH[2]}
elif [[ "$(git describe --tags --match "v*.*.*" --abbrev=0 2>/dev/null)" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] ; then
  # otherwise, find the previous semver tag, extract its information, bump minor/patch, and append -dev
  major_version=${BASH_REMATCH[1]}
  minor_version=${BASH_REMATCH[2]}
  patch_version=${BASH_REMATCH[3]}

  if [[ $short_ref = release-* ]] ; then
    (( patch_version++ ))
  else
    (( minor_version++ ))
  fi

  version=v$major_version.$minor_version.$patch_version-dev
fi

labels=(
  org.opencontainers.image.created="$build_date"
  org.opencontainers.image.licenses="Apache-2.0"
  org.opencontainers.image.revision="$revision"
  org.opencontainers.image.source="https://github.com/$GITHUB_REPOSITORY"
  org.opencontainers.image.url="https://github.com/$GITHUB_REPOSITORY"
  org.opencontainers.image.version="$version"
)

echo "tags=$(IFS=, ; echo "${tags[*]}")" >> "$GITHUB_OUTPUT"
echo "labels=$(IFS=, ; echo "${labels[*]}")" >> "$GITHUB_OUTPUT"

# calculate ldflags for injecting version information into binaries
tree_state="$([[ -z "$(git status --porcelain 2>/dev/null)" ]] && echo clean || echo dirty)"

# passing multi-line strings through an action output is difficult, through env vars is easier
echo "LDFLAGS<<EOF
-X k8s.io/component-base/version.gitMajor=$major_version
-X k8s.io/component-base/version.gitMinor=$minor_version
-X k8s.io/component-base/version.gitVersion=$version
-X k8s.io/component-base/version.gitTreeState=$tree_state
-X k8s.io/component-base/version.gitCommit=$revision
-X k8s.io/component-base/version.buildDate=$build_date
-X k8s.io/component-base/version/verflag.programName=kubernetes-controller-sharding
EOF" >> "$GITHUB_ENV"
