#!/usr/bin/env bash
#
# Copyright 2025 The Kubernetes Authors.
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

# install-config-connector.sh — installs the Config Connector operator on the
# current kubectl context and creates a cluster-mode ConfigConnector resource.
#
# Usage:
#   ./hack/install-config-connector.sh <version>
#
# Prerequisites:
#   - kubectl configured to point at the management cluster
#   - GCP_PROJECT set to the GCP project Config Connector should manage
#   - One of:
#       GCPSA_EMAIL — GCP service account email for key-based auth
#       WORKLOAD_IDENTITY_POOL — Workload Identity pool for key-less auth
#
# Example:
#   GCP_PROJECT=my-project GCPSA_EMAIL=kcc@my-project.iam.gserviceaccount.com \
#     ./hack/install-config-connector.sh 1.125.0

set -o errexit
set -o nounset
set -o pipefail

CONFIG_CONNECTOR_VERSION="${1:?usage: $0 <version>}"

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
KUBECTL="${REPO_ROOT}/hack/tools/bin/kubectl"
cd "${REPO_ROOT}" && make "${KUBECTL##*/}"

GCP_PROJECT="${GCP_PROJECT:?GCP_PROJECT must be set}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

BUNDLE_URL="https://storage.googleapis.com/configconnector-operator/${CONFIG_CONNECTOR_VERSION}/release-bundle.tar.gz"

echo "Downloading Config Connector operator v${CONFIG_CONNECTOR_VERSION}..."
curl --retry 3 -sSL "${BUNDLE_URL}" -o "${TMPDIR}/release-bundle.tar.gz"
tar -zxf "${TMPDIR}/release-bundle.tar.gz" -C "${TMPDIR}"

echo "Installing Config Connector CRDs..."
"${KUBECTL}" apply -f "${TMPDIR}/release-bundle/crds/"

echo "Installing Config Connector operator..."
"${KUBECTL}" apply -f "${TMPDIR}/release-bundle/install-bundle/"

echo "Waiting for Config Connector operator to be ready..."
"${KUBECTL}" wait --for=condition=Available --timeout=5m \
  -n cnrm-system deployment/cnrm-controller-manager 2>/dev/null || \
"${KUBECTL}" wait --for=condition=Ready --timeout=5m \
  -n cnrm-system pod -l cnrm.cloud.google.com/component=cnrm-controller-manager

# Create the ConfigConnector resource in cluster mode.
# Supports key-based auth (GCPSA_EMAIL set) or Workload Identity (WORKLOAD_IDENTITY_POOL set).
if [[ -n "${GCPSA_EMAIL:-}" ]]; then
  echo "Configuring Config Connector with service account key auth (${GCPSA_EMAIL})..."
  "${KUBECTL}" apply -f - <<EOF
apiVersion: core.cnrm.cloud.google.com/v1beta1
kind: ConfigConnector
metadata:
  name: configconnector.core.cnrm.cloud.google.com
spec:
  mode: cluster
  googleServiceAccount: "${GCPSA_EMAIL}"
EOF
elif [[ -n "${WORKLOAD_IDENTITY_POOL:-}" ]]; then
  echo "Configuring Config Connector with Workload Identity (${WORKLOAD_IDENTITY_POOL})..."
  "${KUBECTL}" apply -f - <<EOF
apiVersion: core.cnrm.cloud.google.com/v1beta1
kind: ConfigConnector
metadata:
  name: configconnector.core.cnrm.cloud.google.com
spec:
  mode: cluster
  googleServiceAccount: "${GCPSA_EMAIL:-}"
  workloadIdentityPool: "${WORKLOAD_IDENTITY_POOL}"
EOF
else
  echo "WARNING: Neither GCPSA_EMAIL nor WORKLOAD_IDENTITY_POOL is set."
  echo "         Config Connector operator is installed but NOT configured."
  echo "         Apply a ConfigConnector resource manually before using KCC resources."
  echo "         See: https://cloud.google.com/config-connector/docs/how-to/install-upgrade-uninstall"
  exit 0
fi

echo "Annotating cnrm-system namespace with GCP project..."
"${KUBECTL}" annotate namespace cnrm-system \
  cnrm.cloud.google.com/project-id="${GCP_PROJECT}" --overwrite

echo "Waiting for Config Connector controllers to be ready..."
"${KUBECTL}" wait --for=condition=Ready --timeout=5m \
  -n cnrm-system pod -l cnrm.cloud.google.com/component=cnrm-controller-manager

echo "Config Connector v${CONFIG_CONNECTOR_VERSION} installed successfully."
echo "GCP project: ${GCP_PROJECT}"
