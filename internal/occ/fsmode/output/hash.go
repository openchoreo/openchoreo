// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/pkg/hash"
)

// CompareReleaseSpecs compares two releases and returns true if their specs are identical.
// This uses the shared hash package which implements the same hashing algorithm
// used by the controller for consistency.
//
// To ensure consistent comparison between generated releases (which may have Go int types)
// and file-loaded releases (which have float64 types from YAML parsing), we normalize
// both specs by serializing to YAML and back before hashing.
func CompareReleaseSpecs(release1, release2 *unstructured.Unstructured) (bool, error) {
	spec1, err := extractAndNormalizeSpec(release1)
	if err != nil {
		return false, fmt.Errorf("failed to extract spec from first release (kind=%s, namespace=%s, name=%s): %w",
			release1.GetKind(), release1.GetNamespace(), release1.GetName(), err)
	}

	spec2, err := extractAndNormalizeSpec(release2)
	if err != nil {
		return false, fmt.Errorf("failed to extract spec from second release (kind=%s, namespace=%s, name=%s): %w",
			release2.GetKind(), release2.GetNamespace(), release2.GetName(), err)
	}

	return hash.Equal(spec1, spec2), nil
}

// extractAndNormalizeSpec extracts the spec from an unstructured ComponentRelease
// and normalizes it by round-tripping through YAML.
// This ensures consistent types (e.g., all numbers become float64) regardless of
// whether the release was loaded from a file or generated programmatically.
func extractAndNormalizeSpec(release *unstructured.Unstructured) (interface{}, error) {
	if release == nil {
		return nil, fmt.Errorf("release cannot be nil")
	}

	spec, found := release.Object["spec"]
	if !found {
		return nil, fmt.Errorf("spec not found in release")
	}

	// Normalize by round-tripping through YAML
	// This converts all Go int types to float64, matching YAML parsing behavior
	yamlBytes, err := yaml.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	var normalized interface{}
	if err := yaml.Unmarshal(yamlBytes, &normalized); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return normalized, nil
}
