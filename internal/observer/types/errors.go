// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package types

// Error codes for the new API
const (
	// Logs API (v1) internal server error codes.
	ErrorCodeV1LogsInternalGeneric = "OBS-V1-L-01"
	ErrorCodeV1LogsAuthzInternal   = "OBS-V1-L-02"
	ErrorCodeV1LogsServiceNotReady = "OBS-V1-L-03"
	ErrorCodeV1LogsResolverFailed  = "OBS-V1-L-04"
	ErrorCodeV1LogsRetrievalFailed = "OBS-V1-L-05"

	// Events API (v1) internal server error codes.
	ErrorCodeV1EventsInternalGeneric = "OBS-V1-E-01"
	ErrorCodeV1EventsAuthzInternal   = "OBS-V1-E-02"
	ErrorCodeV1EventsServiceNotReady = "OBS-V1-E-03"
	ErrorCodeV1EventsResolverFailed  = "OBS-V1-E-04"
	ErrorCodeV1EventsRetrievalFailed = "OBS-V1-E-05"
	ErrorCodeV1EventsNotImplemented  = "OBS-V1-E-06"

	// Metrics API (v1) internal server error codes.
	ErrorCodeV1MetricsInternalGeneric = "OBS-V1-M-01"
	ErrorCodeV1MetricsAuthzInternal   = "OBS-V1-M-02"
	ErrorCodeV1MetricsServiceNotReady = "OBS-V1-M-03"
	ErrorCodeV1MetricsResolverFailed  = "OBS-V1-M-04"
	ErrorCodeV1MetricsRetrievalFailed = "OBS-V1-M-05"

	// Traces API (v1alpha1) internal server error codes.
	ErrorCodeV1TracesInternalGeneric = "OBS-V1-T-01"
	ErrorCodeV1TracesAuthzInternal   = "OBS-V1-T-02"
	ErrorCodeV1TracesServiceNotReady = "OBS-V1-T-03"
	ErrorCodeV1TracesResolverFailed  = "OBS-V1-T-04"
	ErrorCodeV1TracesRetrievalFailed = "OBS-V1-T-05"
	ErrorCodeV1TracesInvalidRequest  = "OBS-V1-T-06"
	ErrorCodeV1TracesSpanNotFound    = "OBS-V1-T-07"

	// Runtime topology API (v1alpha1) internal server error codes.
	ErrorCodeV1RuntimeTopologyInternalGeneric = "OBS-V1-RG-01"
	ErrorCodeV1RuntimeTopologyAuthzInternal   = "OBS-V1-RG-02"
	ErrorCodeV1RuntimeTopologyServiceNotReady = "OBS-V1-RG-03"
	ErrorCodeV1RuntimeTopologyResolverFailed  = "OBS-V1-RG-04"
	ErrorCodeV1RuntimeTopologyRetrievalFailed = "OBS-V1-RG-05"

	// Platform logs API (v1alpha1) internal server error codes.
	ErrorCodeV1PlatformLogsInternalGeneric = "OBS-V1-CL-01"
	ErrorCodeV1PlatformLogsAuthzInternal   = "OBS-V1-CL-02"
	ErrorCodeV1PlatformLogsServiceNotReady = "OBS-V1-CL-03"
	ErrorCodeV1PlatformLogsRetrievalFailed = "OBS-V1-CL-04"
	ErrorCodeV1PlatformLogsNotSupported    = "OBS-V1-CL-05"

	// Audit logs API (v1alpha1) internal server error codes.
	ErrorCodeV1AuditLogsInternalGeneric = "OBS-V1-AL-01"
	ErrorCodeV1AuditLogsAuthzInternal   = "OBS-V1-AL-02"
	ErrorCodeV1AuditLogsServiceNotReady = "OBS-V1-AL-03"
	ErrorCodeV1AuditLogsRetrievalFailed = "OBS-V1-AL-04"
	ErrorCodeV1AuditLogsNotSupported    = "OBS-V1-AL-05"
	// ErrorCodeV1AuditLogsCursorExpired accompanies a 400 and means the cursor
	// can no longer be continued. Distinct from a malformed-request 400 because
	// the caller's remedy differs: restart the query rather than fix the body.
	ErrorCodeV1AuditLogsCursorExpired = "OBS-V1-AL-06"
	// ErrorCodeV1AuditLogsFilterValuesNotSupported is distinct from
	// ErrorCodeV1AuditLogsNotSupported: an adapter may serve audit records
	// while being unable to aggregate them, so a client seeing this one should
	// fall back to free-text entry on that filter rather than conclude the
	// trail is unreadable.
	ErrorCodeV1AuditLogsFilterValuesNotSupported = "OBS-V1-AL-07"

	// Scope resolution auth failure — shared across all APIs.
	ErrorCodeV1ScopeAuthFailed = "OBS-V1-SCOPE-AUTH-FAILED"
)
