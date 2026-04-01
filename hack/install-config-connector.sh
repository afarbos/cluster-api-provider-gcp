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

: "${GOOGLE_APPLICATION_CREDENTIALS:?GOOGLE_APPLICATION_CREDENTIALS must be set}"

echo "Installing Config Connector v${VERSION}..."

# Download and install the operator
BUNDLE_URL="https://storage.googleapis.com/configconnector-operator/${VERSION}/release-bundle.tar.gz"
TMPDIR=$(mktemp -d)
trap "rm -rf ${TMPDIR}" EXIT

curl -sL "${BUNDLE_URL}" | tar xz -C "${TMPDIR}"
kubectl apply -f "${TMPDIR}/operator-system/configconnector-operator.yaml"

# Wait for operator statefulset to be ready
echo "Waiting for Config Connector operator..."
kubectl rollout status statefulset/configconnector-operator -n configconnector-operator-system --timeout=300s

# Create credentials secret
kubectl create namespace cnrm-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic "${SECRET_NAME}" \
  --from-file=key.json="${GOOGLE_APPLICATION_CREDENTIALS}" \
  -n cnrm-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Apply cluster-mode ConfigConnector and resource customizations together
# so the operator creates pods with the right limits from the start.
# See: https://cloud.google.com/config-connector/docs/how-to/customizing-container-resources
cat <<EOF | kubectl apply -f -
apiVersion: customize.core.cnrm.cloud.google.com/v1beta1
kind: ControllerResource
metadata:
  name: cnrm-webhook-manager
spec:
  containers:
  - name: webhook
    resources:
      limits:
        memory: 512Mi
      requests:
        memory: 256Mi
---
apiVersion: customize.core.cnrm.cloud.google.com/v1beta1
kind: ControllerResource
metadata:
  name: cnrm-resource-stats-recorder
spec:
  containers:
  - name: recorder
    resources:
      limits:
        memory: 256Mi
      requests:
        memory: 128Mi
---
apiVersion: core.cnrm.cloud.google.com/v1beta1
kind: ConfigConnector
metadata:
  name: configconnector.core.cnrm.cloud.google.com
spec:
  mode: cluster
  credentialSecretName: "${SECRET_NAME}"
EOF

echo "Waiting for Config Connector to be ready..."
kubectl wait --for=jsonpath='{.status.healthy}'=true --timeout=300s configconnector/configconnector.core.cnrm.cloud.google.com

echo "Config Connector v${VERSION} installed successfully."
