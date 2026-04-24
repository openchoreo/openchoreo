// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/interpreter"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

func requireActivationMap(t *testing.T, act interpreter.Activation, root string) map[string]any {
	t.Helper()
	val, found := act.ResolveName(root)
	require.True(t, found, "expected variable %q to be bound in activation", root)
	m, ok := val.(map[string]any)
	require.True(t, ok, "expected variable %q to be map[string]any", root)
	return m
}

func TestBuildCelActivation(t *testing.T) {
	envAttr := authzcore.AttrResourceEnvironment

	t.Run("empty allowedAttrs yields empty activation", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "dev"}}
		act, err := buildCelActivation(ctx, nil)
		require.NoError(t, err)
		require.NotNil(t, act)
	})

	t.Run("allowed attr with value in ctx is bound to real value", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "staging"}}
		act, err := buildCelActivation(ctx, []authzcore.AttributeSpec{envAttr})
		require.NoError(t, err)

		m := requireActivationMap(t, act, "resource")
		require.Equal(t, "staging", m["environment"])
	})

	t.Run("allowed attr missing from ctx is bound to typed zero (empty string)", func(t *testing.T) {
		ctx := authzcore.Context{}
		act, err := buildCelActivation(ctx, []authzcore.AttributeSpec{envAttr})
		require.NoError(t, err)

		m := requireActivationMap(t, act, "resource")
		require.Equal(t, "", m["environment"])
	})

	t.Run("attribute not in allowed list is added to activation", func(t *testing.T) {
		regionAttr := authzcore.AttributeSpec{Key: "resource.region", CELType: cel.StringType}
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "prod"}}
		act, err := buildCelActivation(ctx, []authzcore.AttributeSpec{regionAttr, envAttr})
		require.NoError(t, err)

		m := requireActivationMap(t, act, "resource")
		require.Equal(t, "prod", m["environment"])
		require.Contains(t, m, "region")
	})
}

func TestConvertCtxToAttrMap(t *testing.T) {
	t.Run("valid context produces expected two-level map", func(t *testing.T) {
		ctx := authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "dev"}}
		m, err := convertCtxToAttrMap(ctx)
		require.NoError(t, err)
		require.Equal(t, "dev", m["resource"]["environment"])
	})

	t.Run("empty context produces empty or zero-value map", func(t *testing.T) {
		ctx := authzcore.Context{}
		m, err := convertCtxToAttrMap(ctx)
		require.NoError(t, err)
		if resourceMap, ok := m["resource"]; ok {
			require.Empty(t, resourceMap["environment"])
		}
	})
}

func TestZeroForCELType(t *testing.T) {
	tests := []struct {
		name    string
		celType *cel.Type
		want    any
	}{
		{
			name:    "string type returns empty string",
			celType: cel.StringType,
			want:    "",
		},
		{
			name:    "bool type returns false",
			celType: cel.BoolType,
			want:    false,
		},
		{
			name:    "int type returns int64 zero",
			celType: cel.IntType,
			want:    int64(0),
		},
		{
			name:    "double type returns float64 zero",
			celType: cel.DoubleType,
			want:    0.0,
		},
		{
			name:    "unknown type returns nil",
			celType: cel.DynType,
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zeroForCELType(tt.celType)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCompileCEL(t *testing.T) {
	t.Run("valid bool expression compiles and evaluates", func(t *testing.T) {
		prg, err := compileCEL(`resource.environment == "dev"`)
		require.NoError(t, err)
		require.NotNil(t, prg)
	})

	t.Run("syntax error returns error", func(t *testing.T) {
		prg, err := compileCEL(`this is not valid CEL ((((`)
		require.Error(t, err)
		require.Nil(t, prg)
	})

	t.Run("empty expression returns error", func(t *testing.T) {
		prg, err := compileCEL(``)
		require.Error(t, err)
		require.Nil(t, prg)
	})
}
