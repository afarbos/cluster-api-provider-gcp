#!/usr/bin/env bash
# Install Config Connector on a Kubernetes cluster.
# Usage: ./hack/install-config-connector.sh [VERSION]
#
# Required env vars:
#   GCP_PROJECT - GCP project ID
#   GOOGLE_APPLICATION_CREDENTIALS - path to service account key JSON
#
# Optional env vars:
#   KCC_CREDENTIALS_SECRET - secret name (default: gcp-key)

set -euo pipefail

VERSION="${1:-1.146.0}"
SECRET_NAME="${KCC_CREDENTIALS_SECRET:-gcp-key}"

: "${GCP_PROJECT:?GCP_PROJECT must be set}"
: "${GOOGLE_APPLICATION_CREDENTIALS:?GOOGLE_APPLICATION_CREDENTIALS must be set}"

echo "Installing Config Connector v${VERSION}..."

# Download and install the operator
BUNDLE_URL="https://storage.googleapis.com/configconnector-operator/${VERSION}/release-bundle.tar.gz"
TMPDIR=$(mktemp -d)
trap "rm -rf ${TMPDIR}" EXIT

curl -sL "${BUNDLE_URL}" | tar xz -C "${TMPDIR}"
kubectl apply -f "${TMPDIR}/operator-system/configconnector-operator.yaml"

# Wait for operator to be ready
echo "Waiting for Config Connector operator..."
kubectl wait --for=condition=Available --timeout=300s deployment/configconnector-operator -n configconnector-operator-system

# Create credentials secret
kubectl create namespace cnrm-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic "${SECRET_NAME}" \
  --from-file=key.json="${GOOGLE_APPLICATION_CREDENTIALS}" \
  -n cnrm-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Apply cluster-mode ConfigConnector
cat <<EOF | kubectl apply -f -
apiVersion: core.cnrm.cloud.google.com/v1beta1
kind: ConfigConnector
metadata:
  name: configconnector.core.cnrm.cloud.google.com
spec:
  mode: cluster
  googleServiceAccount: ""
  credentialSecretName: "${SECRET_NAME}"
EOF

# Apply ConfigConnectorContext for the default namespace
cat <<EOF | kubectl apply -f -
apiVersion: core.cnrm.cloud.google.com/v1beta1
kind: ConfigConnectorContext
metadata:
  name: configconnectorcontext.core.cnrm.cloud.google.com
  namespace: default
spec:
  googleServiceAccount: ""
  requestTimeout: 20m
EOF

echo "Waiting for Config Connector to be ready..."
kubectl wait --for=condition=Healthy --timeout=300s configconnector/configconnector.core.cnrm.cloud.google.com || true

echo "Config Connector v${VERSION} installed successfully."
