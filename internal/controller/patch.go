// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasPatchChanges determines if two Kubernetes objects have differences by comparing their patch data.
func HasPatchChanges(original, desired client.Object) (bool, []byte, error) {
	patch := client.MergeFrom(original)
	patchData, err := patch.Data(desired)
	if err != nil {
		return false, nil, fmt.Errorf("failed to generate patch: %w", err)
	}

	// Empty patch ("{}") means no changes needed
	changed := !bytes.Equal(patchData, []byte("{}"))
	return changed, patchData, nil
}

// Merge applies a strategic merge patch to the base object using the overlay object.
// This function uses Kubernetes strategic merge patch semantics to combine two objects of the same type.
//
// Parameters:
//   - base: the original object to merge into
//   - overlay: the object containing changes to apply
//
// Returns:
//   - *T: the merged result object, or nil if either input is nil
//   - error: any error that occurred during marshaling, patching, or unmarshaling
//
// Example usage:
//
//	base := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
//	overlay := &v1.Pod{Spec: v1.PodSpec{RestartPolicy: v1.RestartPolicyAlways}}
//	merged, err := controller.Merge(base, overlay)
//	if err != nil {
//	    return fmt.Errorf("failed to merge: %w", err)
//	}
func Merge[T any](base, overlay *T) (*T, error) {
	if base == nil || overlay == nil {
		return nil, nil
	}

	origJSON, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	patchJSON, err := json.Marshal(overlay)
	if err != nil {
		return nil, err
	}

	var zero T
	mergedJSON, err := strategicpatch.StrategicMergePatch(origJSON, patchJSON, zero)
	if err != nil {
		return nil, err
	}

	var merged T
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}
