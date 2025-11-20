// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"strings"
)

// WorkloadType represents the type of workload based on ComponentType
type WorkloadType string

const (
	WorkloadTypeDeployment  WorkloadType = "deployment"
	WorkloadTypeStatefulSet WorkloadType = "statefulset"
	WorkloadTypeCronJob     WorkloadType = "cronjob"
	WorkloadTypeJob         WorkloadType = "job"
	WorkloadTypeUnknown     WorkloadType = "unknown"
)

// extractWorkloadType extracts the workload type from ComponentType field.
// ComponentType format: "deployment/http-service", "cronjob/scheduled-task", etc.
// The pattern is validated as: ^(deployment|statefulset|cronjob|job)/[a-z0-9]([-a-z0-9]*[a-z0-9])?$
func extractWorkloadType(componentType string) WorkloadType {
	if componentType == "" {
		return WorkloadTypeUnknown
	}

	// Split by "/" and take the first part
	parts := strings.SplitN(componentType, "/", 2)
	if len(parts) == 0 {
		return WorkloadTypeUnknown
	}

	switch parts[0] {
	case "deployment":
		return WorkloadTypeDeployment
	case "statefulset":
		return WorkloadTypeStatefulSet
	case "cronjob":
		return WorkloadTypeCronJob
	case "job":
		return WorkloadTypeJob
	default:
		return WorkloadTypeUnknown
	}
}
