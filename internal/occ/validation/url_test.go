// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid URL",
			val:     "https://example.com/path",
			wantErr: false,
		},
		{
			name:    "empty string",
			val:     "",
			wantErr: true,
			errMsg:  "empty field",
		},
		{
			name:    "non-string type",
			val:     42,
			wantErr: true,
			errMsg:  "invalid type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.val)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateGitHubURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid GitHub URL",
			url:     "https://github.com/owner/repo",
			wantErr: false,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
			errMsg:  "required",
		},
		{
			name:    "non-GitHub URL",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
			errMsg:  "only GitHub URLs",
		},
		{
			name:    "missing owner (no slash in path)",
			url:     "https://github.com/repo",
			wantErr: true,
			errMsg:  "invalid GitHub repository format",
		},
		{
			name:    "too many segments",
			url:     "https://github.com/owner/repo/extra",
			wantErr: true,
			errMsg:  "invalid GitHub repository format",
		},
		{
			name:    "trailing slash creates extra segment",
			url:     "https://github.com/owner/repo/",
			wantErr: true,
			errMsg:  "invalid GitHub repository format",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGitHubURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}
