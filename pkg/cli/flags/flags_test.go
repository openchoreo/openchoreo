// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddFlags(t *testing.T) {
	tests := []struct {
		name     string
		flags    []Flag
		wantType string // "string", "bool", "int", "stringArray"
		check    func(t *testing.T, cmd *cobra.Command)
	}{
		{
			name: "string flag with shorthand",
			flags: []Flag{
				{Name: "project", Shorthand: "p", Usage: "project name"},
			},
			check: func(t *testing.T, cmd *cobra.Command) {
				f := cmd.Flags().Lookup("project")
				require.NotNil(t, f)
				assert.Equal(t, "string", f.Value.Type())
				assert.Equal(t, "p", f.ShorthandDeprecated+f.Shorthand)
				assert.Equal(t, "project name", f.Usage)
				assert.Equal(t, "", f.DefValue)
			},
		},
		{
			name: "string flag without shorthand",
			flags: []Flag{
				{Name: "kubeconfig", Usage: "path to kubeconfig"},
			},
			check: func(t *testing.T, cmd *cobra.Command) {
				f := cmd.Flags().Lookup("kubeconfig")
				require.NotNil(t, f)
				assert.Equal(t, "string", f.Value.Type())
				assert.Equal(t, "", f.Shorthand)
			},
		},
		{
			name: "bool flag",
			flags: []Flag{
				{Name: "follow", Shorthand: "f", Usage: "follow logs", Type: "bool"},
			},
			check: func(t *testing.T, cmd *cobra.Command) {
				f := cmd.Flags().Lookup("follow")
				require.NotNil(t, f)
				assert.Equal(t, "bool", f.Value.Type())
				assert.Equal(t, "f", f.Shorthand)
				assert.Equal(t, "false", f.DefValue)
			},
		},
		{
			name: "int flag",
			flags: []Flag{
				{Name: "tail", Usage: "number of lines", Type: "int"},
			},
			check: func(t *testing.T, cmd *cobra.Command) {
				f := cmd.Flags().Lookup("tail")
				require.NotNil(t, f)
				assert.Equal(t, "int", f.Value.Type())
				assert.Equal(t, "0", f.DefValue)
			},
		},
		{
			name: "stringArray flag",
			flags: []Flag{
				{Name: "set", Usage: "set values", Type: "stringArray"},
			},
			check: func(t *testing.T, cmd *cobra.Command) {
				f := cmd.Flags().Lookup("set")
				require.NotNil(t, f)
				assert.Equal(t, "stringArray", f.Value.Type())
				assert.Equal(t, "[]", f.DefValue)
			},
		},
		{
			name: "multiple flags at once",
			flags: []Flag{
				{Name: "namespace", Shorthand: "n", Usage: "namespace"},
				{Name: "output", Shorthand: "o", Usage: "output format"},
				{Name: "follow", Type: "bool", Usage: "follow"},
			},
			check: func(t *testing.T, cmd *cobra.Command) {
				assert.NotNil(t, cmd.Flags().Lookup("namespace"))
				assert.NotNil(t, cmd.Flags().Lookup("output"))
				assert.NotNil(t, cmd.Flags().Lookup("follow"))
			},
		},
		{
			name:  "no flags",
			flags: []Flag{},
			check: func(t *testing.T, cmd *cobra.Command) {
				// Should not panic; command has no custom flags
				assert.False(t, cmd.Flags().HasFlags())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			AddFlags(cmd, tt.flags...)
			tt.check(t, cmd)
		})
	}
}
