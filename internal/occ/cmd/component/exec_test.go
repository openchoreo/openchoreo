// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func TestBuildExecWebSocketURL(t *testing.T) {
	tests := []struct {
		name       string
		base       string
		params     ExecParams
		wantScheme string
		wantPath   string
		wantKeys   []string // query keys that must be present
	}{
		{
			name: "http converts to ws",
			base: "http://localhost:8080",
			params: ExecParams{
				Namespace: "default",
				Component: "my-service",
			},
			wantScheme: "ws://",
			wantPath:   "/exec/namespaces/default/components/my-service",
		},
		{
			name: "https converts to wss",
			base: "https://api.example.com",
			params: ExecParams{
				Namespace: "ns",
				Component: "comp",
			},
			wantScheme: "wss://",
			wantPath:   "/exec/namespaces/ns/components/comp",
		},
		{
			name: "all flags set",
			base: "http://localhost:8080",
			params: ExecParams{
				Namespace:   "acme",
				Project:     "store",
				Component:   "api",
				Environment: "dev",
				Container:   "app",
				TTY:         true,
				Stdin:       true,
				Command:     []string{"echo", "hello world"},
			},
			wantScheme: "ws://",
			wantPath:   "/exec/namespaces/acme/components/api",
			wantKeys:   []string{"project", "env", "container", "tty", "stdin", "command"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildExecWebSocketURL(tt.base, tt.params)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(got, tt.wantScheme),
				"expected scheme %q, got URL %q", tt.wantScheme, got)
			assert.Contains(t, got, tt.wantPath)
			for _, key := range tt.wantKeys {
				assert.Contains(t, got, key+"=")
			}
		})
	}
}

func TestNewExecCmdArgParsing(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantComp   string
		wantCmdLen int // expected length of command after --
	}{
		{
			name:       "component name only, no --",
			args:       []string{"my-service"},
			wantComp:   "my-service",
			wantCmdLen: 0,
		},
		{
			name:       "component with -- separator and single command",
			args:       []string{"my-service", "--", "ls"},
			wantComp:   "my-service",
			wantCmdLen: 1,
		},
		{
			name:       "component with -- separator and multi-word command",
			args:       []string{"my-service", "--", "echo", "hello", "world"},
			wantComp:   "my-service",
			wantCmdLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedComp string
			var capturedCmd []string

			cmd := newExecCmd(func() (client.Interface, error) { return nil, nil })
			cmd.PreRunE = nil
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				capturedComp = args[0]
				if dash := cmd.ArgsLenAtDash(); dash > 0 {
					capturedCmd = args[dash:]
				}
				return nil
			}
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tt.wantComp, capturedComp)
			assert.Equal(t, tt.wantCmdLen, len(capturedCmd))
		})
	}
}
