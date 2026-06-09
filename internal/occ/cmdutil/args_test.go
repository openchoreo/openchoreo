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
