// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// makePipelineUnstructured builds an unstructured DeploymentPipeline with the given promotion paths.
func makePipelineUnstructured(name string, paths []map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "core.choreo.dev/v1alpha1",
			"kind":       "DeploymentPipeline",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	if paths != nil {
		obj.Object["spec"] = map[string]interface{}{
			"promotionPaths": toInterfaceSlice(paths),
		}
	}
	return obj
}

func toInterfaceSlice(paths []map[string]interface{}) []interface{} {
	result := make([]interface{}, len(paths))
	for i, p := range paths {
		result[i] = p
	}
	return result
}

func pathMap(source string, targets ...string) map[string]interface{} {
	targetRefs := make([]interface{}, len(targets))
	for i, t := range targets {
		targetRefs[i] = map[string]interface{}{"name": t}
	}
	return map[string]interface{}{
		"sourceEnvironmentRef":  map[string]interface{}{"name": source},
		"targetEnvironmentRefs": targetRefs,
	}
}

// linearPipeline creates a dev -> staging -> prod pipeline for reuse.
func linearPipeline() *unstructured.Unstructured {
	return makePipelineUnstructured("test-pipeline", []map[string]interface{}{
		pathMap("dev", "staging"),
		pathMap("staging", "prod"),
	})
}

func TestParsePipeline(t *testing.T) {
	t.Run("valid linear pipeline", func(t *testing.T) {
		info, err := ParsePipeline(linearPipeline())
		require.NoError(t, err)
		assert.Equal(t, "test-pipeline", info.Name)
		assert.Equal(t, "dev", info.RootEnvironment)
		assert.Contains(t, info.Environments, "dev")
		assert.Contains(t, info.Environments, "staging")
		assert.Contains(t, info.Environments, "prod")
		assert.Equal(t, []string{"staging"}, info.PromotionPaths["dev"])
		assert.Equal(t, []string{"prod"}, info.PromotionPaths["staging"])
	})

	t.Run("diamond shape", func(t *testing.T) {
		p := makePipelineUnstructured("diamond", []map[string]interface{}{
			pathMap("dev", "staging-a", "staging-b"),
			pathMap("staging-a", "prod"),
			pathMap("staging-b", "prod"),
		})
		info, err := ParsePipeline(p)
		require.NoError(t, err)
		assert.Equal(t, "dev", info.RootEnvironment)
		assert.Equal(t, 0, info.EnvPosition["dev"])
		assert.Equal(t, 1, info.EnvPosition["staging-a"])
		assert.Equal(t, 1, info.EnvPosition["staging-b"])
		assert.Equal(t, 2, info.EnvPosition["prod"])
	})

	t.Run("nil pipeline", func(t *testing.T) {
		_, err := ParsePipeline(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("no promotion paths", func(t *testing.T) {
		p := makePipelineUnstructured("empty", nil)
		_, err := ParsePipeline(p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no promotion paths")
	})

	t.Run("malformed paths skipped", func(t *testing.T) {
		p := makePipelineUnstructured("partial", []map[string]interface{}{
			pathMap("dev", "staging"),
			{"bad": "entry"},
		})
		info, err := ParsePipeline(p)
		require.NoError(t, err)
		assert.Equal(t, "dev", info.RootEnvironment)
	})
}

func TestFindRootEnvironment(t *testing.T) {
	t.Run("single root", func(t *testing.T) {
		root, err := FindRootEnvironment(linearPipeline())
		require.NoError(t, err)
		assert.Equal(t, "dev", root)
	})

	t.Run("all sources are targets", func(t *testing.T) {
		p := makePipelineUnstructured("cycle", []map[string]interface{}{
			pathMap("a", "b"),
			pathMap("b", "a"),
		})
		_, err := FindRootEnvironment(p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no root environment")
	})

	t.Run("nil pipeline", func(t *testing.T) {
		_, err := FindRootEnvironment(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("no paths", func(t *testing.T) {
		p := makePipelineUnstructured("empty", nil)
		_, err := FindRootEnvironment(p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no promotion paths")
	})
}

func TestValidateEnvironment(t *testing.T) {
	info, err := ParsePipeline(linearPipeline())
	require.NoError(t, err)

	t.Run("valid environment", func(t *testing.T) {
		assert.NoError(t, info.ValidateEnvironment("staging"))
	})

	t.Run("empty name", func(t *testing.T) {
		err := info.ValidateEnvironment("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("unknown environment", func(t *testing.T) {
		err := info.ValidateEnvironment("nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}

func TestIsRootEnvironment(t *testing.T) {
	info, err := ParsePipeline(linearPipeline())
	require.NoError(t, err)

	t.Run("root", func(t *testing.T) {
		assert.True(t, info.IsRootEnvironment("dev"))
	})

	t.Run("non-root", func(t *testing.T) {
		assert.False(t, info.IsRootEnvironment("staging"))
	})
}

func TestGetPreviousEnvironment(t *testing.T) {
	info, err := ParsePipeline(linearPipeline())
	require.NoError(t, err)

	t.Run("root returns empty", func(t *testing.T) {
		prev, err := info.GetPreviousEnvironment("dev")
		require.NoError(t, err)
		assert.Empty(t, prev)
	})

	t.Run("valid target", func(t *testing.T) {
		prev, err := info.GetPreviousEnvironment("staging")
		require.NoError(t, err)
		assert.Equal(t, "dev", prev)
	})

	t.Run("unknown env", func(t *testing.T) {
		_, err := info.GetPreviousEnvironment("unknown")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := info.GetPreviousEnvironment("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})
}

func TestGetEnvironmentPosition(t *testing.T) {
	info, err := ParsePipeline(linearPipeline())
	require.NoError(t, err)

	t.Run("root is 0", func(t *testing.T) {
		pos, err := info.GetEnvironmentPosition("dev")
		require.NoError(t, err)
		assert.Equal(t, 0, pos)
	})

	t.Run("second is 1", func(t *testing.T) {
		pos, err := info.GetEnvironmentPosition("staging")
		require.NoError(t, err)
		assert.Equal(t, 1, pos)
	})

	t.Run("unknown returns error", func(t *testing.T) {
		_, err := info.GetEnvironmentPosition("unknown")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}

func TestExtractSourceEnvironmentRefName(t *testing.T) {
	t.Run("object format", func(t *testing.T) {
		pathMap := map[string]interface{}{
			"sourceEnvironmentRef": map[string]interface{}{"name": "dev", "kind": "Environment"},
		}
		assert.Equal(t, "dev", extractSourceEnvironmentRefName(pathMap))
	})

	t.Run("legacy string format", func(t *testing.T) {
		pathMap := map[string]interface{}{
			"sourceEnvironmentRef": "staging",
		}
		assert.Equal(t, "staging", extractSourceEnvironmentRefName(pathMap))
	})

	t.Run("nil ref", func(t *testing.T) {
		pathMap := map[string]interface{}{}
		assert.Equal(t, "", extractSourceEnvironmentRefName(pathMap))
	})
}
