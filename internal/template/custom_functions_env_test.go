// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template_test

import (
	"fmt"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	celcontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
)

// This file lives in the external test package because it builds the same CEL
// environments the component and resource validation webhooks build, and those packages
// import internal/template.

// The lists.range ceiling comes from re-declaring cel-go's own lists_range overload, so it
// holds only while that re-declaration is the last one to win. Anything that appends
// ext.Lists() after BaseCELExtensions, or that declares lists.range a second time, either
// restores the unbounded builtin or fails env construction outright. The render engine's
// own env is covered in package template; these are the other environments that admit
// tenant-authored CEL, and each builds its env rather than reusing the engine's.
func TestListsRangeCeilingHoldsInEveryEnvironment(t *testing.T) {
	t.Parallel()

	const maxRange = template.MaxListsRangeSizeForTest

	envs := map[string]func() (*cel.Env, error){
		// Mirrors createBaseEnv in internal/validation/component/cel_env.go.
		"component validation": func() (*cel.Env, error) {
			opts := template.BaseCELExtensions()
			opts = append(opts, celcontext.CELValidationExtensions()...)
			return cel.NewEnv(opts...)
		},
		// Mirrors createBaseEnv in internal/validation/resource/cel_env.go.
		"resource validation": func() (*cel.Env, error) {
			return cel.NewEnv(template.BaseCELExtensions()...)
		},
		// Mirrors the component render surface, which adds the non-validation helpers.
		"component render": func() (*cel.Env, error) {
			opts := template.BaseCELExtensions()
			opts = append(opts, celcontext.CELExtensions()...)
			return cel.NewEnv(opts...)
		},
	}

	for name, build := range envs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env, err := build()
			require.NoError(t, err, "a duplicate lists.range declaration would fail env construction here")

			out, err := evalRange(t, env, maxRange)
			require.NoError(t, err, "a range at the cap must still materialize")
			assert.Equal(t, maxRange, listLen(t, out))

			_, err = evalRange(t, env, maxRange+1)
			require.Error(t, err, "the bounded overload must win over cel-go's builtin")
			assert.Contains(t, err.Error(), "exceeds maximum allowed")
		})
	}
}

// evalRange compiles and runs lists.range(size) against env.
func evalRange(t *testing.T, env *cel.Env, size int64) (ref.Val, error) {
	t.Helper()
	compiled, iss := env.Compile(fmt.Sprintf("lists.range(%d)", size))
	require.NoError(t, iss.Err())
	prg, err := env.Program(compiled)
	require.NoError(t, err)
	val, _, err := prg.Eval(map[string]any{})
	return val, err
}

func listLen(t *testing.T, val ref.Val) int64 {
	t.Helper()
	sizer, ok := val.(traits.Sizer)
	require.True(t, ok, "expected a sizable list, got %T", val)
	n, ok := sizer.Size().(types.Int)
	require.True(t, ok, "expected an integer size, got %v", sizer.Size())
	return int64(n)
}
