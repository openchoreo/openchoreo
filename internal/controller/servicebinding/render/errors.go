// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func UnsupportedWorkloadTypeError(workloadType openchoreov1alpha1.WorkloadType) error {
	return fmt.Errorf("unsupported workload type: %s", workloadType)
}

func MergeError(err error) error {
	return fmt.Errorf("failed to merge: %w", err)
}
