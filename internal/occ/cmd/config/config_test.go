// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/testutil"
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

// --- CRUD operation tests ---
// These tests exercise the Config methods that read/write via the on-disk
// YAML config, using setupTestHome to isolate each test.

// seedConfig is a helper that writes a pre-populated StoredConfig to the test home.
func seedConfig(t *testing.T, cfg *StoredConfig) {
	t.Helper()
	require.NoError(t, SaveStoredConfig(cfg))
}

func TestListContexts(t *testing.T) {
	c := New()

	t.Run("prints message when no contexts stored", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListContexts())
		})
		assert.Contains(t, out, "No contexts stored.")
	})

	t.Run("lists contexts with headers", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts: []Context{
				{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1", Namespace: "ns1", Project: "proj1", Component: "comp1"},
				{Name: "ctx2", ControlPlane: "cp2", Credentials: "cred2"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListContexts())
		})
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "CONTROLPLANE")
		assert.Contains(t, out, "CREDENTIALS")
		assert.Contains(t, out, "ctx1")
		assert.Contains(t, out, "ctx2")
		assert.Contains(t, out, "cp1")
		assert.Contains(t, out, "ns1")
	})

	t.Run("marks current context with asterisk", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts: []Context{
				{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1"},
				{Name: "ctx2", ControlPlane: "cp1", Credentials: "cred1"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListContexts())
		})
		assert.Contains(t, out, "*")
	})
}

func TestListControlPlanes(t *testing.T) {
	c := New()

	t.Run("prints message when no control planes stored", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListControlPlanes())
		})
		assert.Contains(t, out, "No control planes stored.")
	})

	t.Run("lists control planes with headers", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			ControlPlanes: []ControlPlane{
				{Name: "cp1", URL: "http://localhost:8080"},
				{Name: "cp2", URL: "http://remote:9090"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListControlPlanes())
		})
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "URL")
		assert.Contains(t, out, "cp1")
		assert.Contains(t, out, "http://localhost:8080")
		assert.Contains(t, out, "cp2")
		assert.Contains(t, out, "http://remote:9090")
	})
}

func TestListCredentials(t *testing.T) {
	c := New()

	t.Run("prints message when no credentials stored", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListCredentials())
		})
		assert.Contains(t, out, "No credentials stored.")
	})

	t.Run("lists credentials with headers", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Credentials: []Credential{
				{Name: "cred1", AuthMethod: "pkce", Token: "tok123"},
				{Name: "cred2", AuthMethod: "pat"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListCredentials())
		})
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "AUTH METHOD")
		assert.Contains(t, out, "HAS TOKEN")
		assert.Contains(t, out, "cred1")
		assert.Contains(t, out, "pkce")
		assert.Contains(t, out, "yes")
		assert.Contains(t, out, "cred2")
	})

	t.Run("shows dash for credential without token", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Credentials: []Credential{
				{Name: "cred1", AuthMethod: "pkce"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			require.NoError(t, c.ListCredentials())
		})
		assert.Contains(t, out, "-")
		assert.NotContains(t, out, "yes")
	})
}

func TestAddContext(t *testing.T) {
	c := New()

	t.Run("creates context with new CP and credential", func(t *testing.T) {
		setupTestHome(t)

		out := testutil.CaptureStdout(t, func() {
			err := c.AddContext(AddContextParams{
				Name:         "ctx1",
				ControlPlane: "cp1",
				Credentials:  "cred1",
				Namespace:    "ns1",
				Project:      "proj1",
			})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Created context: ctx1")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Contexts, 1)
		assert.Equal(t, "ctx1", cfg.Contexts[0].Name)
		assert.Equal(t, "ns1", cfg.Contexts[0].Namespace)
		// CP and credential should be auto-created
		require.Len(t, cfg.ControlPlanes, 1)
		assert.Equal(t, "cp1", cfg.ControlPlanes[0].Name)
		require.Len(t, cfg.Credentials, 1)
		assert.Equal(t, "cred1", cfg.Credentials[0].Name)
	})

	t.Run("reuses existing CP and credential", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			ControlPlanes: []ControlPlane{{Name: "cp1", URL: "http://localhost"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.AddContext(AddContextParams{
				Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1",
			})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Created context: ctx1")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		// Should not duplicate
		assert.Len(t, cfg.ControlPlanes, 1)
		assert.Len(t, cfg.Credentials, 1)
	})

	t.Run("rejects duplicate context name", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:      []Context{{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1"}},
			ControlPlanes: []ControlPlane{{Name: "cp1"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		err := c.AddContext(AddContextParams{
			Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestDeleteContext(t *testing.T) {
	c := New()

	t.Run("deletes existing non-current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts: []Context{
				{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1"},
				{Name: "ctx2", ControlPlane: "cp1", Credentials: "cred1"},
			},
			ControlPlanes: []ControlPlane{{Name: "cp1"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.DeleteContext(DeleteContextParams{Name: "ctx2"})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Deleted context: ctx2")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Len(t, cfg.Contexts, 1)
		assert.Equal(t, "ctx1", cfg.Contexts[0].Name)
	})

	t.Run("rejects deleting current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1"}},
		})

		err := c.DeleteContext(DeleteContextParams{Name: "ctx1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete the current context")
	})

	t.Run("returns error for non-existent context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		err := c.DeleteContext(DeleteContextParams{Name: "ghost"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestUpdateContext(t *testing.T) {
	c := New()

	t.Run("updates namespace and project", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:      []Context{{Name: "ctx1", Namespace: "old-ns", Project: "old-proj", ControlPlane: "cp1", Credentials: "cred1"}},
			ControlPlanes: []ControlPlane{{Name: "cp1"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.UpdateContext(UpdateContextParams{
				Name: "ctx1", Namespace: "new-ns", Project: "new-proj",
			})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Updated context: ctx1")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "new-ns", cfg.Contexts[0].Namespace)
		assert.Equal(t, "new-proj", cfg.Contexts[0].Project)
		// ControlPlane and Credentials should remain unchanged
		assert.Equal(t, "cp1", cfg.Contexts[0].ControlPlane)
		assert.Equal(t, "cred1", cfg.Contexts[0].Credentials)
	})

	t.Run("returns error for non-existent context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		err := c.UpdateContext(UpdateContextParams{Name: "ghost", Namespace: "ns"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("rejects non-existent control plane reference", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:      []Context{{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1"}},
			ControlPlanes: []ControlPlane{{Name: "cp1"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		err := c.UpdateContext(UpdateContextParams{Name: "ctx1", ControlPlane: "no-such-cp"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("rejects non-existent credentials reference", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:      []Context{{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1"}},
			ControlPlanes: []ControlPlane{{Name: "cp1"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		err := c.UpdateContext(UpdateContextParams{Name: "ctx1", Credentials: "no-such-cred"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "do not exist")
	})
}

func TestUseContext(t *testing.T) {
	c := New()

	t.Run("switches current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts: []Context{
				{Name: "ctx1"},
				{Name: "ctx2"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.UseContext(UseContextParams{Name: "ctx2"})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Now using context: ctx2")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "ctx2", cfg.CurrentContext)
	})

	t.Run("returns error for non-existent context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts: []Context{{Name: "ctx1"}},
		})

		err := c.UseContext(UseContextParams{Name: "ghost"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestAddControlPlane(t *testing.T) {
	c := New()

	t.Run("creates control plane", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		out := testutil.CaptureStdout(t, func() {
			err := c.AddControlPlane(AddControlPlaneParams{
				Name: "cp1", URL: "http://localhost:8080",
			})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Created control plane: cp1")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		require.Len(t, cfg.ControlPlanes, 1)
		assert.Equal(t, "cp1", cfg.ControlPlanes[0].Name)
		assert.Equal(t, "http://localhost:8080", cfg.ControlPlanes[0].URL)
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			ControlPlanes: []ControlPlane{{Name: "cp1", URL: "http://localhost"}},
		})

		err := c.AddControlPlane(AddControlPlaneParams{Name: "cp1", URL: "http://other"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("rejects empty URL", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		err := c.AddControlPlane(AddControlPlaneParams{Name: "cp1", URL: "   "})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "URL must not be empty")
	})
}

func TestUpdateControlPlane(t *testing.T) {
	c := New()

	t.Run("updates URL", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			ControlPlanes: []ControlPlane{{Name: "cp1", URL: "http://old"}},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.UpdateControlPlane(UpdateControlPlaneParams{
				Name: "cp1", URL: "http://new",
			})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Updated control plane: cp1")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "http://new", cfg.ControlPlanes[0].URL)
	})

	t.Run("returns error for non-existent control plane", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		err := c.UpdateControlPlane(UpdateControlPlaneParams{Name: "ghost", URL: "http://x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestDeleteControlPlane(t *testing.T) {
	c := New()

	t.Run("deletes unreferenced control plane", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			ControlPlanes: []ControlPlane{
				{Name: "cp1", URL: "http://a"},
				{Name: "cp2", URL: "http://b"},
			},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.DeleteControlPlane(DeleteControlPlaneParams{Name: "cp2"})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Deleted control plane: cp2")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Len(t, cfg.ControlPlanes, 1)
	})

	t.Run("rejects deleting referenced control plane", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:      []Context{{Name: "ctx1", ControlPlane: "cp1"}},
			ControlPlanes: []ControlPlane{{Name: "cp1", URL: "http://a"}},
		})

		err := c.DeleteControlPlane(DeleteControlPlaneParams{Name: "cp1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "referenced by context")
	})

	t.Run("returns error for non-existent control plane", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		err := c.DeleteControlPlane(DeleteControlPlaneParams{Name: "ghost"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestAddCredentials(t *testing.T) {
	c := New()

	t.Run("creates credential", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		out := testutil.CaptureStdout(t, func() {
			err := c.AddCredentials(AddCredentialsParams{Name: "cred1"})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Created credentials: cred1")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Credentials, 1)
		assert.Equal(t, "cred1", cfg.Credentials[0].Name)
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Credentials: []Credential{{Name: "cred1"}},
		})

		err := c.AddCredentials(AddCredentialsParams{Name: "cred1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestDeleteCredentials(t *testing.T) {
	c := New()

	t.Run("deletes unreferenced credential", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Credentials: []Credential{{Name: "cred1"}, {Name: "cred2"}},
		})

		out := testutil.CaptureStdout(t, func() {
			err := c.DeleteCredentials(DeleteCredentialsParams{Name: "cred2"})
			require.NoError(t, err)
		})
		assert.Contains(t, out, "Deleted credentials: cred2")

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Len(t, cfg.Credentials, 1)
	})

	t.Run("rejects deleting referenced credential", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:    []Context{{Name: "ctx1", Credentials: "cred1"}},
			Credentials: []Credential{{Name: "cred1"}},
		})

		err := c.DeleteCredentials(DeleteCredentialsParams{Name: "cred1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "referenced by context")
	})

	t.Run("returns error for non-existent credential", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		err := c.DeleteCredentials(DeleteCredentialsParams{Name: "ghost"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetCurrentContext(t *testing.T) {
	t.Run("returns current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", Namespace: "ns1", ControlPlane: "cp1"}},
		})

		ctx, err := GetCurrentContext()
		require.NoError(t, err)
		assert.Equal(t, "ctx1", ctx.Name)
		assert.Equal(t, "ns1", ctx.Namespace)
	})

	t.Run("returns error when no current context set", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})

		_, err := GetCurrentContext()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no current context set")
	})

	t.Run("returns error when current context not found", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ghost",
			Contexts:       []Context{{Name: "ctx1"}},
		})

		_, err := GetCurrentContext()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetCurrentCredential(t *testing.T) {
	t.Run("returns credential for current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", Credentials: "cred1", ControlPlane: "cp1"}},
			Credentials:    []Credential{{Name: "cred1", Token: "tok123", AuthMethod: "pkce"}},
			ControlPlanes:  []ControlPlane{{Name: "cp1"}},
		})

		cred, err := GetCurrentCredential()
		require.NoError(t, err)
		assert.Equal(t, "cred1", cred.Name)
		assert.Equal(t, "tok123", cred.Token)
	})

	t.Run("returns error when no credentials on context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", ControlPlane: "cp1"}},
			ControlPlanes:  []ControlPlane{{Name: "cp1"}},
		})

		_, err := GetCurrentCredential()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no credentials associated")
	})

	t.Run("returns error when credential not found", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", Credentials: "ghost", ControlPlane: "cp1"}},
			ControlPlanes:  []ControlPlane{{Name: "cp1"}},
		})

		_, err := GetCurrentCredential()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetCurrentControlPlane(t *testing.T) {
	t.Run("returns control plane for current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", ControlPlane: "cp1"}},
			ControlPlanes:  []ControlPlane{{Name: "cp1", URL: "http://localhost:8080"}},
		})

		cp, err := GetCurrentControlPlane()
		require.NoError(t, err)
		assert.Equal(t, "cp1", cp.Name)
		assert.Equal(t, "http://localhost:8080", cp.URL)
	})

	t.Run("returns error when control plane not found", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", ControlPlane: "ghost"}},
		})

		_, err := GetCurrentControlPlane()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestEnsureContext(t *testing.T) {
	t.Run("creates default config when none exists", func(t *testing.T) {
		setupTestHome(t)
		// Set default env values
		t.Setenv("CHOREO_API_ENDPOINT", "http://test:9090")

		err := EnsureContext()
		require.NoError(t, err)

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "default", cfg.CurrentContext)
		require.Len(t, cfg.Contexts, 1)
		assert.Equal(t, "default", cfg.Contexts[0].Name)
		require.Len(t, cfg.ControlPlanes, 1)
		assert.Equal(t, "http://test:9090", cfg.ControlPlanes[0].URL)
		require.Len(t, cfg.Credentials, 1)
	})

	t.Run("does nothing when config already exists", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "my-ctx",
			Contexts:       []Context{{Name: "my-ctx"}},
			ControlPlanes:  []ControlPlane{{Name: "my-cp", URL: "http://existing"}},
		})

		err := EnsureContext()
		require.NoError(t, err)

		// Original config should be untouched
		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "my-ctx", cfg.CurrentContext)
		assert.Len(t, cfg.Contexts, 1)
		assert.Equal(t, "http://existing", cfg.ControlPlanes[0].URL)
	})
}

func TestApplyContextDefaults(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("namespace", "", "")
		cmd.Flags().String("project", "", "")
		cmd.Flags().String("component", "", "")
		return cmd
	}

	t.Run("skips for config subcommand", func(t *testing.T) {
		setupTestHome(t)
		parent := &cobra.Command{Use: "config"}
		cmd := &cobra.Command{Use: "test"}
		parent.AddCommand(cmd)
		require.NoError(t, ApplyContextDefaults(cmd))
	})

	t.Run("returns nil when no current context", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{})
		require.NoError(t, ApplyContextDefaults(newCmd()))
	})

	t.Run("returns error when current context not found", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ghost",
			Contexts:       []Context{{Name: "ctx1"}},
		})
		err := ApplyContextDefaults(newCmd())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("applies context defaults to unset flags", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts: []Context{{
				Name:      "ctx1",
				Namespace: "ns1",
				Project:   "proj1",
				Component: "comp1",
			}},
		})
		cmd := newCmd()
		require.NoError(t, ApplyContextDefaults(cmd))
		assert.Equal(t, "ns1", cmd.Flags().Lookup("namespace").Value.String())
		assert.Equal(t, "proj1", cmd.Flags().Lookup("project").Value.String())
		assert.Equal(t, "comp1", cmd.Flags().Lookup("component").Value.String())
	})

	t.Run("does not override explicitly set flags", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", Namespace: "from-config", Project: "from-config"}},
		})
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("namespace", "explicit"))
		require.NoError(t, ApplyContextDefaults(cmd))
		assert.Equal(t, "explicit", cmd.Flags().Lookup("namespace").Value.String())
		assert.Equal(t, "from-config", cmd.Flags().Lookup("project").Value.String())
	})
}

func TestUpdateContext_ComponentField(t *testing.T) {
	c := New()

	t.Run("updates component field", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			Contexts:      []Context{{Name: "ctx1", ControlPlane: "cp1", Credentials: "cred1", Component: "old-comp"}},
			ControlPlanes: []ControlPlane{{Name: "cp1"}},
			Credentials:   []Credential{{Name: "cred1"}},
		})

		require.NoError(t, c.UpdateContext(UpdateContextParams{Name: "ctx1", Component: "new-comp"}))

		cfg, err := LoadStoredConfig()
		require.NoError(t, err)
		assert.Equal(t, "new-comp", cfg.Contexts[0].Component)
	})
}

func TestUpdateControlPlane_WhitespaceURL(t *testing.T) {
	c := New()

	t.Run("rejects whitespace-only URL", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			ControlPlanes: []ControlPlane{{Name: "cp1", URL: "http://old"}},
		})

		err := c.UpdateControlPlane(UpdateControlPlaneParams{Name: "cp1", URL: "   "})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})
}

func TestGetCurrentControlPlane_NoControlPlane(t *testing.T) {
	t.Run("returns error when context has no control plane match", func(t *testing.T) {
		setupTestHome(t)
		seedConfig(t, &StoredConfig{
			CurrentContext: "ctx1",
			Contexts:       []Context{{Name: "ctx1", ControlPlane: "ghost"}},
			ControlPlanes:  []ControlPlane{{Name: "cp1"}},
		})

		_, err := GetCurrentControlPlane()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetControlPlaneAndCredentials(t *testing.T) {
	t.Run("GetControlPlane returns control plane name", func(t *testing.T) {
		p := AddContextParams{ControlPlane: "my-cp"}
		assert.Equal(t, "my-cp", p.GetControlPlane())
	})

	t.Run("GetCredentials returns credentials name", func(t *testing.T) {
		p := AddContextParams{Credentials: "my-cred"}
		assert.Equal(t, "my-cred", p.GetCredentials())
	})
}
