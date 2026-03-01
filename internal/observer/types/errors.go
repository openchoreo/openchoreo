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
)
