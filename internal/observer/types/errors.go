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
)
