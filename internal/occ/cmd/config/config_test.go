// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{
			name:  "empty token",
			token: "",
			want:  "***",
		},
		{
			name:  "short token (<=8 chars)",
			token: "abcd1234",
			want:  "***",
		},
		{
			name:  "normal token shows first4...last4",
			token: "abcdefghijklmnop",
			want:  "abcd...mnop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskToken(tt.token)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateAddContextParams(t *testing.T) {
	tests := []struct {
		name    string
		params  AddContextParams
		wantErr string
	}{
		{
			name: "valid params",
			params: AddContextParams{
				Name:         "my-context",
				ControlPlane: "my-cp",
				Credentials:  "my-creds",
			},
			wantErr: "",
		},
		{
			name: "empty name",
			params: AddContextParams{
				Name:         "",
				ControlPlane: "my-cp",
				Credentials:  "my-creds",
			},
			wantErr: "name is required",
		},
		{
			name: "empty control plane",
			params: AddContextParams{
				Name:         "my-context",
				ControlPlane: "",
				Credentials:  "my-creds",
			},
			wantErr: "control plane name is required",
		},
		{
			name: "empty credentials",
			params: AddContextParams{
				Name:         "my-context",
				ControlPlane: "my-cp",
				Credentials:  "",
			},
			wantErr: "credentials name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAddContextParams(tt.params)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidateContextNameUniqueness(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *StoredConfig
		ctxName string
		wantErr string
	}{
		{
			name:    "unique name",
			cfg:     &StoredConfig{Contexts: []Context{{Name: "existing"}}},
			ctxName: "new-context",
			wantErr: "",
		},
		{
			name:    "duplicate name",
			cfg:     &StoredConfig{Contexts: []Context{{Name: "existing"}}},
			ctxName: "existing",
			wantErr: `context "existing" already exists`,
		},
		{
			name:    "empty list",
			cfg:     &StoredConfig{},
			ctxName: "any-name",
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContextNameUniqueness(tt.cfg, tt.ctxName)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidateControlPlaneNameUniqueness(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *StoredConfig
		cpName  string
		wantErr string
	}{
		{
			name:    "unique name",
			cfg:     &StoredConfig{ControlPlanes: []ControlPlane{{Name: "existing-cp"}}},
			cpName:  "new-cp",
			wantErr: "",
		},
		{
			name:    "duplicate name",
			cfg:     &StoredConfig{ControlPlanes: []ControlPlane{{Name: "existing-cp"}}},
			cpName:  "existing-cp",
			wantErr: `control plane "existing-cp" already exists`,
		},
		{
			name:    "empty list",
			cfg:     &StoredConfig{},
			cpName:  "any-cp",
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateControlPlaneNameUniqueness(tt.cfg, tt.cpName)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		envVar       string
		envValue     string
		setEnv       bool
		defaultValue string
		want         string
	}{
		{
			name:         "returns env value when set",
			envVar:       "TEST_CHOREO_VAR",
			envValue:     "from-env",
			setEnv:       true,
			defaultValue: "fallback",
			want:         "from-env",
		},
		{
			name:         "returns default when env not set",
			envVar:       "TEST_CHOREO_UNSET",
			setEnv:       false,
			defaultValue: "fallback",
			want:         "fallback",
		},
		{
			name:         "returns default when env is empty string",
			envVar:       "TEST_CHOREO_EMPTY",
			envValue:     "",
			setEnv:       true,
			defaultValue: "fallback",
			want:         "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.envVar, tt.envValue)
			}
			got := getEnvOrDefault(tt.envVar, tt.defaultValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetDefaultContextValues(t *testing.T) {
	t.Run("returns defaults when no env vars set", func(t *testing.T) {
		t.Setenv("CHOREO_DEFAULT_CONTEXT", "")
		t.Setenv("CHOREO_DEFAULT_ORG", "")
		t.Setenv("CHOREO_DEFAULT_PROJECT", "")
		t.Setenv("CHOREO_DEFAULT_CREDENTIAL", "")
		t.Setenv("CHOREO_DEFAULT_CONTROLPLANE", "")

		defaults := getDefaultContextValues()
		assert.Equal(t, "default", defaults.ContextName)
		assert.Equal(t, "default", defaults.Namespace)
		assert.Equal(t, "default", defaults.Project)
		assert.Equal(t, "default", defaults.Credentials)
		assert.Equal(t, "default", defaults.ControlPlane)
	})

	t.Run("returns env values when set", func(t *testing.T) {
		t.Setenv("CHOREO_DEFAULT_CONTEXT", "my-ctx")
		t.Setenv("CHOREO_DEFAULT_ORG", "my-org")
		t.Setenv("CHOREO_DEFAULT_PROJECT", "my-proj")
		t.Setenv("CHOREO_DEFAULT_CREDENTIAL", "my-cred")
		t.Setenv("CHOREO_DEFAULT_CONTROLPLANE", "my-cp")

		defaults := getDefaultContextValues()
		assert.Equal(t, "my-ctx", defaults.ContextName)
		assert.Equal(t, "my-org", defaults.Namespace)
		assert.Equal(t, "my-proj", defaults.Project)
		assert.Equal(t, "my-cred", defaults.Credentials)
		assert.Equal(t, "my-cp", defaults.ControlPlane)
	})
}

func TestValidateCredentialsNameUniqueness(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *StoredConfig
		credName string
		wantErr  string
	}{
		{
			name:     "unique name",
			cfg:      &StoredConfig{Credentials: []Credential{{Name: "existing-cred"}}},
			credName: "new-cred",
			wantErr:  "",
		},
		{
			name:     "duplicate name",
			cfg:      &StoredConfig{Credentials: []Credential{{Name: "existing-cred"}}},
			credName: "existing-cred",
			wantErr:  `credentials "existing-cred" already exists`,
		},
		{
			name:     "empty list",
			cfg:      &StoredConfig{},
			credName: "any-cred",
			wantErr:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentialsNameUniqueness(tt.cfg, tt.credName)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
