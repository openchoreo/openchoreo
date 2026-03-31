// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	ChoreoGroup = "openchoreo.dev"
)

const (
	LabelNamespace = "openchoreo.dev/namespace"
	LabelProject   = "openchoreo.dev/project"
	LabelName      = "openchoreo.dev/name"
)
const (
	AnnotationDescription = "openchoreo.dev/description"
	AnnotationDisplayName = "openchoreo.dev/display-name"
)

type APIVersion string

const (
	V1Alpha1 APIVersion = "v1alpha1"
)

type CRDConfig struct {
	Group   string
	Version APIVersion
	Kind    string
}

var (
	WorkloadV1Config = CRDConfig{
		Group:   ChoreoGroup,
		Version: V1Alpha1,
		Kind:    "Workload",
	}
)
