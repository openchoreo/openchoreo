// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForEachType_String(t *testing.T) {
	assert.Equal(t, "list", forEachList.String())
	assert.Equal(t, "map", forEachMap.String())
	assert.Equal(t, "unknown", forEachUnknown.String())
}

func TestAnalyzeForEachExpression(t *testing.T) {
	env, _, err := buildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)

	t.Run("default varName is item", func(t *testing.T) {
		info, err := analyzeForEachExpression(`["a","b"]`, "", env)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "item", info.VarName)
	})

	t.Run("custom varName preserved", func(t *testing.T) {
		info, err := analyzeForEachExpression(`["a","b"]`, "elem", env)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "elem", info.VarName)
	})

	t.Run("list expression returns forEachList", func(t *testing.T) {
		info, err := analyzeForEachExpression(`["a","b","c"]`, "item", env)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, forEachList, info.Type)
		assert.NotNil(t, info.ElementType)
	})

	t.Run("map expression returns forEachMap", func(t *testing.T) {
		info, err := analyzeForEachExpression(`{"key1": "val1", "key2": "val2"}`, "item", env)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, forEachMap, info.Type)
		assert.NotNil(t, info.KeyType)
		assert.NotNil(t, info.ValueType)
	})

	t.Run("parse error returns error", func(t *testing.T) {
		_, err := analyzeForEachExpression("@@@", "item", env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid forEach expression")
	})

	t.Run("type check error returns unknown type", func(t *testing.T) {
		// Reference an undeclared variable — type-check fails but should return unknown
		info, err := analyzeForEachExpression("undeclaredVar", "item", env)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, forEachUnknown, info.Type)
		assert.Equal(t, cel.DynType, info.VarType)
	})

	t.Run("workload.endpoints returns forEachMap", func(t *testing.T) {
		info, err := analyzeForEachExpression("workload.endpoints", "ep", env)
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, forEachMap, info.Type)
	})
}

func TestExtendEnvWithForEach(t *testing.T) {
	env, _, err := buildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)
	validator, err := NewCELValidator(ComponentTypeResource, SchemaOptions{})
	require.NoError(t, err)

	t.Run("nil info returns env unchanged", func(t *testing.T) {
		result, err := extendEnvWithForEach(env, nil, validator.GetTypeProvider())
		require.NoError(t, err)
		assert.Equal(t, env, result)
	})

	t.Run("list forEach extends with variable", func(t *testing.T) {
		info := &forEachInfo{
			Type:        forEachList,
			VarName:     "item",
			VarType:     cel.StringType,
			ElementType: cel.StringType,
		}
		result, err := extendEnvWithForEach(env, info, validator.GetTypeProvider())
		require.NoError(t, err)
		// Verify the variable is available in the extended env
		_, issues := result.Compile("item")
		assert.Nil(t, issues.Err())
	})

	t.Run("unknown forEach extends with DynType", func(t *testing.T) {
		info := &forEachInfo{
			Type:    forEachUnknown,
			VarName: "item",
			VarType: cel.DynType,
		}
		result, err := extendEnvWithForEach(env, info, validator.GetTypeProvider())
		require.NoError(t, err)
		_, issues := result.Compile("item")
		assert.Nil(t, issues.Err())
	})

	t.Run("map forEach extends with MapEntry type", func(t *testing.T) {
		info := &forEachInfo{
			Type:      forEachMap,
			VarName:   "entry",
			VarType:   cel.DynType,
			KeyType:   cel.StringType,
			ValueType: cel.DynType,
		}
		result, err := extendEnvWithForEach(env, info, validator.GetTypeProvider())
		require.NoError(t, err)
		// Verify key and value fields are accessible
		_, issues := result.Compile("entry.key")
		assert.Nil(t, issues.Err())
		_, issues = result.Compile("entry.value")
		assert.Nil(t, issues.Err())
	})
}

func TestResolveValueDeclType(t *testing.T) {
	_, provider, err := buildComponentCELEnv(SchemaOptions{})
	require.NoError(t, err)

	t.Run("nil valueType returns DynType", func(t *testing.T) {
		result := resolveValueDeclType(nil, provider)
		assert.NotNil(t, result)
	})

	t.Run("nil provider returns DynType", func(t *testing.T) {
		result := resolveValueDeclType(cel.StringType, nil)
		assert.NotNil(t, result)
	})

	t.Run("non-struct kind returns DynType", func(t *testing.T) {
		result := resolveValueDeclType(cel.StringType, provider)
		assert.NotNil(t, result)
	})
}
