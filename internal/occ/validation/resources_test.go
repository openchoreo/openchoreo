// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		val     any
		wantErr bool
		errMsg  string
	}{
		{name: "valid name", val: "my-resource"},
		{name: "empty string", val: "", wantErr: true, errMsg: "empty field"},
		{name: "non-string type", val: 123, wantErr: true, errMsg: "invalid type"},
		{name: "leading hyphen", val: "-foo", wantErr: true, errMsg: "invalid"},
		{name: "trailing hyphen", val: "foo-", wantErr: true, errMsg: "invalid"},
		{name: "uppercase", val: "MyResource", wantErr: true, errMsg: "invalid"},
		{name: "single char", val: "a", wantErr: true, errMsg: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName("resource", tt.val)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateNamespaceName(t *testing.T) {
	require.NoError(t, ValidateNamespaceName("my-ns"))

	err := ValidateNamespaceName("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace")
}

func TestValidateProjectName(t *testing.T) {
	require.NoError(t, ValidateProjectName("my-proj"))

	err := ValidateProjectName("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project")
}

func TestValidateComponentName(t *testing.T) {
	require.NoError(t, ValidateComponentName("my-comp"))

	err := ValidateComponentName("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "component")
}
