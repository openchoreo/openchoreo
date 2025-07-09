// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/strategicpatch"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// merge applies a strategic merge patch to the base object using the overlay object.
func merge[T any](base, overlay *T) (*T, error) {
	if base == nil && overlay == nil {
		return nil, nil
	}
	if base == nil {
		return overlay, nil
	}
	if overlay == nil {
		return base, nil
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

// MergePoliciesForExposeLevel merges the default policies with the expose level specific policies
// Returns the final policy to be applied for the given expose level
func MergePoliciesForExposeLevel(restPolicy *openchoreov1alpha1.RESTAPIPolicy, exposeLevel openchoreov1alpha1.RESTOperationExposeLevel) (*openchoreov1alpha1.RESTPolicyWithConditionals, error) {
	if restPolicy == nil {
		return nil, nil
	}

	// Start with defaults as base
	mergedPolicy := restPolicy.Defaults

	// Apply expose level specific overrides
	var exposeLevelPolicy *openchoreov1alpha1.RESTPolicyWithConditionals
	switch exposeLevel {
	case openchoreov1alpha1.ExposeLevelPublic:
		exposeLevelPolicy = restPolicy.Public
	case openchoreov1alpha1.ExposeLevelOrganization:
		exposeLevelPolicy = restPolicy.Organization
	}

	// Merge expose level policy on top of defaults
	if exposeLevelPolicy != nil {
		var err error
		mergedPolicy, err = merge(mergedPolicy, exposeLevelPolicy)
		if err != nil {
			return nil, err
		}
	}

	return mergedPolicy, nil
}
