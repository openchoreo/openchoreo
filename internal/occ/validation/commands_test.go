// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"
	"testing"
)

func TestCheckRequiredFields(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]string
		want   bool
	}{
		{
			name:   "all fields set",
			fields: map[string]string{"namespace": "ns", "name": "foo"},
			want:   true,
		},
		{
			name:   "one empty field",
			fields: map[string]string{"namespace": "ns", "name": ""},
			want:   false,
		},
		{
			name:   "all empty fields",
			fields: map[string]string{"namespace": "", "name": ""},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkRequiredFields(tt.fields); got != tt.want {
				t.Errorf("checkRequiredFields() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateHelpError(t *testing.T) {
	tests := []struct {
		name        string
		cmdType     CommandType
		resource    ResourceType
		fields      map[string]string
		wantSubstrs []string
	}{
		{
			name:     "single missing field",
			cmdType:  CmdCreate,
			resource: ResourceProject,
			fields:   map[string]string{"name": ""},
			wantSubstrs: []string{
				"Missing required parameter:",
				"--name",
				"occ create project -h",
			},
		},
		{
			name:     "multiple missing fields",
			cmdType:  CmdCreate,
			resource: ResourceProject,
			fields:   map[string]string{"namespace": "", "name": ""},
			wantSubstrs: []string{
				"Missing required parameters:",
			},
		},
		{
			name:     "empty resource",
			cmdType:  CmdLogs,
			resource: "",
			fields:   map[string]string{"type": ""},
			wantSubstrs: []string{
				"occ logs -h",
			},
		},
		{
			name:     "plural check single vs multiple",
			cmdType:  CmdGet,
			resource: ResourceComponent,
			fields:   map[string]string{"namespace": ""},
			wantSubstrs: []string{
				"Missing required parameter:",
				"--namespace",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := generateHelpError(tt.cmdType, tt.resource, tt.fields)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			msg := err.Error()
			for _, sub := range tt.wantSubstrs {
				if !strings.Contains(msg, sub) {
					t.Errorf("error %q does not contain %q", msg, sub)
				}
			}
		})
	}
}

func TestPluralS(t *testing.T) {
	if got := pluralS(1); got != "" {
		t.Errorf("pluralS(1) = %q, want %q", got, "")
	}
	if got := pluralS(2); got != "s" {
		t.Errorf("pluralS(2) = %q, want %q", got, "s")
	}
}
