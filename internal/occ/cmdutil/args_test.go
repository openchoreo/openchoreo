// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractArgName(t *testing.T) {
	tests := []struct {
		use  string
		want string
	}{
		{"get [NAMESPACE_NAME]", "NAMESPACE_NAME"},
		{"run WORKFLOW_NAME", "WORKFLOW_NAME"},
		{"delete [NAME]", "NAME"},
		{"list", "NAME"},   // no arg part → fallback
		{"", "NAME"},       // empty → fallback
		{"cmd []", "NAME"}, // empty brackets → fallback
	}
	for _, tt := range tests {
		t.Run(tt.use, func(t *testing.T) {
			assert.Equal(t, tt.want, extractArgName(tt.use))
		})
	}
}

func TestExactOneArgWithUsage(t *testing.T) {
	newCmd := func(use string) *cobra.Command {
		return &cobra.Command{Use: use}
	}

	t.Run("exactly one arg passes", func(t *testing.T) {
		cmd := newCmd("get [NAMESPACE_NAME]")
		validator := ExactOneArgWithUsage()
		require.NoError(t, validator(cmd, []string{"my-ns"}))
	})

	t.Run("no args returns descriptive error with arg name", func(t *testing.T) {
		cmd := newCmd("get [NAMESPACE_NAME]")
		validator := ExactOneArgWithUsage()
		err := validator(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NAMESPACE_NAME")
		assert.Contains(t, err.Error(), "required argument")
	})

	t.Run("no args with no use arg falls back to NAME", func(t *testing.T) {
		cmd := newCmd("list")
		validator := ExactOneArgWithUsage()
		err := validator(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NAME")
	})

	t.Run("too many args returns count error", func(t *testing.T) {
		cmd := newCmd("get [NAME]")
		validator := ExactOneArgWithUsage()
		err := validator(cmd, []string{"a", "b"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg(s), received 2")
	})
}

func TestExactArgsWithUsage(t *testing.T) {
	cmd := &cobra.Command{Use: "diff [FROM_RELEASE] [TO_RELEASE]"}
	validator := ExactArgsWithUsage(2)

	t.Run("exactly two args passes", func(t *testing.T) {
		assert.NoError(t, validator(cmd, []string{"a", "b"}))
	})

	t.Run("no args names both", func(t *testing.T) {
		err := validator(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required arguments FROM_RELEASE and TO_RELEASE not provided")
		assert.Contains(t, err.Error(), "Usage:")
	})

	t.Run("one arg names the second", func(t *testing.T) {
		err := validator(cmd, []string{"a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required argument TO_RELEASE not provided")
	})

	t.Run("three args", func(t *testing.T) {
		err := validator(cmd, []string{"a", "b", "c"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 2 arg(s), received 3")
	})
}

func TestExtractArgNames(t *testing.T) {
	assert.Equal(t, []string{"FROM", "TO"}, extractArgNames("diff [FROM] TO", 2))
	assert.Equal(t, []string{"FROM", "ARG2"}, extractArgNames("diff FROM", 2))
	assert.Equal(t, []string{"ARG1", "ARG2"}, extractArgNames("", 2))
}
