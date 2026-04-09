// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

// APIVersion represents a Kubernetes API version string.
type APIVersion = string

const (
	ChoreoGroup            = "core.choreo.dev"
	V1Alpha1    APIVersion = "v1alpha1"
)

// CRDConfig holds the GroupVersionKind for a CRD.
type CRDConfig struct {
	Group   string
	Version APIVersion
	Kind    string
}

// WorkloadV1Config is the default CRDConfig for Workload resources.
var WorkloadV1Config = CRDConfig{
	Group:   ChoreoGroup,
	Version: V1Alpha1,
	Kind:    "Workload",
}
