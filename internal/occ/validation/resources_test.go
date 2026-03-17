// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid name",
			val:     "my-resource",
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
			val:     123,
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name:    "leading hyphen",
			val:     "-foo",
			wantErr: true,
			errMsg:  "invalid",
		},
		{
			name:    "trailing hyphen",
			val:     "foo-",
			wantErr: true,
			errMsg:  "invalid",
		},
		{
			name:    "uppercase",
			val:     "MyResource",
			wantErr: true,
			errMsg:  "invalid",
		},
		{
			name:    "single char",
			val:     "a",
			wantErr: true,
			errMsg:  "invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName("resource", tt.val)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateNamespaceName(t *testing.T) {
	if err := ValidateNamespaceName("my-ns"); err != nil {
		t.Errorf("expected no error for valid name, got %v", err)
	}
	err := ValidateNamespaceName("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Errorf("error %q should contain 'namespace'", err.Error())
	}
}

func TestValidateProjectName(t *testing.T) {
	if err := ValidateProjectName("my-proj"); err != nil {
		t.Errorf("expected no error for valid name, got %v", err)
	}
	err := ValidateProjectName("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("error %q should contain 'project'", err.Error())
	}
}

func TestValidateComponentName(t *testing.T) {
	if err := ValidateComponentName("my-comp"); err != nil {
		t.Errorf("expected no error for valid name, got %v", err)
	}
	err := ValidateComponentName("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "component") {
		t.Errorf("error %q should contain 'component'", err.Error())
	}
}
