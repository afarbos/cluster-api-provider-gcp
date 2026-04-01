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

package v1beta2

// KCC condition types and reasons.
const (
	// KCCDegradedCondition indicates the resource is in a degraded state.
	KCCDegradedCondition = "Degraded"

	// KCCDeletionBlockedCondition indicates deletion is blocked.
	KCCDeletionBlockedCondition = "DeletionBlocked"

	// KCC condition reasons (KCC-specific only; use clusterv1 constants for
	// generic reasons like Ready, NotReady, Deleting, etc.).
	KCCResourceCreatingReason      = "KCCResourceCreating"
	KCCDeletionTimeoutReason       = "DeletionTimeout"
	KCCReconciliationTimeoutReason = "ReconciliationTimeout"

	// ConfigurationErrorReason indicates a user configuration error prevented reconciliation.
	ConfigurationErrorReason = "ConfigurationError"
)
