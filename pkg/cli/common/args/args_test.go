// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package args

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestExactOneArgWithUsage(t *testing.T) {
	tests := []struct {
		name    string
		use     string
		args    []string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no args with bracketed name",
			use:     "get [NAMESPACE_NAME]",
			args:    []string{},
			wantErr: true,
			errMsg:  "required argument NAMESPACE_NAME not provided",
		},
		{
			name:    "no args with unbracketed name",
			use:     "run WORKFLOW_NAME",
			args:    []string{},
			wantErr: true,
			errMsg:  "required argument WORKFLOW_NAME not provided",
		},
		{
			name:    "no args with no arg name in use",
			use:     "scaffold",
			args:    []string{},
			wantErr: true,
			errMsg:  "required argument NAME not provided",
		},
		{
			name:    "one arg succeeds",
			use:     "get [NAMESPACE_NAME]",
			args:    []string{"my-ns"},
			wantErr: false,
		},
		{
			name:    "too many args",
			use:     "get [NAMESPACE_NAME]",
			args:    []string{"a", "b"},
			wantErr: true,
			errMsg:  "accepts 1 arg(s), received 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: tt.use}
			err := ExactOneArgWithUsage()(cmd, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExtractArgName(t *testing.T) {
	tests := []struct {
		use  string
		want string
	}{
		{"get [NAMESPACE_NAME]", "NAMESPACE_NAME"},
		{"delete [PROJECT_NAME]", "PROJECT_NAME"},
		{"run WORKFLOW_NAME", "WORKFLOW_NAME"},
		{"scaffold COMPONENT_NAME", "COMPONENT_NAME"},
		{"list", "NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.use, func(t *testing.T) {
			got := extractArgName(tt.use)
			if got != tt.want {
				t.Errorf("extractArgName(%q) = %q, want %q", tt.use, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
