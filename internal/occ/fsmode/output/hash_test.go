// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeRelease(name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "core.choreo.dev/v1alpha1",
		"kind":       "ComponentRelease",
		"metadata":   map[string]interface{}{"name": name},
	}
	if spec != nil {
		obj["spec"] = spec
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestCompareReleaseSpecs(t *testing.T) {
	t.Run("identical specs", func(t *testing.T) {
		r1 := makeRelease("r1", map[string]interface{}{"image": "nginx", "replicas": 3})
		r2 := makeRelease("r2", map[string]interface{}{"image": "nginx", "replicas": 3})
		equal, err := CompareReleaseSpecs(r1, r2)
		require.NoError(t, err)
		assert.True(t, equal)
	})

	t.Run("different specs", func(t *testing.T) {
		r1 := makeRelease("r1", map[string]interface{}{"image": "nginx", "replicas": 3})
		r2 := makeRelease("r2", map[string]interface{}{"image": "nginx", "replicas": 5})
		equal, err := CompareReleaseSpecs(r1, r2)
		require.NoError(t, err)
		assert.False(t, equal)
	})

	t.Run("missing spec", func(t *testing.T) {
		r1 := makeRelease("r1", map[string]interface{}{"image": "nginx"})
		r2 := makeRelease("r2", nil)
		_, err := CompareReleaseSpecs(r1, r2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec not found")
	})

	t.Run("nil release returns error", func(t *testing.T) {
		r1 := makeRelease("r1", map[string]interface{}{"image": "nginx"})
		_, err := CompareReleaseSpecs(r1, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "release cannot be nil")
	})
}

func TestExtractAndNormalizeSpec(t *testing.T) {
	t.Run("valid spec", func(t *testing.T) {
		r := makeRelease("r1", map[string]interface{}{"image": "nginx", "replicas": 3})
		spec, err := extractAndNormalizeSpec(r)
		require.NoError(t, err)
		require.NotNil(t, spec)
		specMap, ok := spec.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "nginx", specMap["image"])
	})

	t.Run("missing spec", func(t *testing.T) {
		r := makeRelease("r1", nil)
		_, err := extractAndNormalizeSpec(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec not found")
	})

	t.Run("nil release", func(t *testing.T) {
		_, err := extractAndNormalizeSpec(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})
}
