/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

const (
	// KCC condition types for v1beta2 conditions (string type).
	KCCNetworkReadyCondition    = "KCCNetworkReady"
	KCCSubnetworkReadyCondition = "KCCSubnetworkReady"
	KCCClusterReadyCondition    = "KCCClusterReady"
	KCCNodePoolReadyCondition   = "KCCNodePoolReady"
	KCCDegradedCondition        = "Degraded"
	KCCDeletionBlockedCondition = "DeletionBlocked"

	// KCC condition reasons (KCC-specific only; use clusterv1 constants for
	// generic reasons like Ready, NotReady, Deleting, etc.).
	KCCResourceCreatingReason      = "KCCResourceCreating"
	KCCDeletionTimeoutReason       = "DeletionTimeout"
	KCCReconciliationTimeoutReason = "ReconciliationTimeout"
)
