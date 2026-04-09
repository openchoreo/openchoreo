// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func TestNewLogoutCmd(t *testing.T) {
	cmd := NewLogoutCmd()
	assert.Equal(t, "logout", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewLogoutCmd_RunE(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://localhost"}},
		Credentials: []config.Credential{{
			Name:         "cred",
			Token:        "tok",
			RefreshToken: "rtok",
		}},
		Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	})

	cmd := NewLogoutCmd()
	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	cfg, err := config.LoadStoredConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Credentials[0].Token)
	assert.Empty(t, cfg.Credentials[0].RefreshToken)
}

func TestNewLogoutImpl(t *testing.T) {
	impl := NewLogoutImpl()
	assert.NotNil(t, impl)
}

func TestLogout(t *testing.T) {
	t.Run("clears token and refresh token for current credential", func(t *testing.T) {
		home := testutil.SetupTestHome(t)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://localhost"}},
			Credentials: []config.Credential{{
				Name:         "cred",
				Token:        "some-token",
				RefreshToken: "some-refresh-token",
				AuthMethod:   "authorization_code",
			}},
			Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		require.NoError(t, NewLogoutImpl().Logout())

		cfg, err := config.LoadStoredConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Credentials, 1)
		assert.Empty(t, cfg.Credentials[0].Token)
		assert.Empty(t, cfg.Credentials[0].RefreshToken)
	})

	t.Run("leaves other credentials untouched", func(t *testing.T) {
		home := testutil.SetupTestHome(t)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://localhost"}},
			Credentials: []config.Credential{
				{Name: "cred", Token: "token-a", RefreshToken: "refresh-a"},
				{Name: "other", Token: "token-b", RefreshToken: "refresh-b"},
			},
			Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		require.NoError(t, NewLogoutImpl().Logout())

		cfg, err := config.LoadStoredConfig()
		require.NoError(t, err)

		var cred, other config.Credential
		for _, c := range cfg.Credentials {
			switch c.Name {
			case "cred":
				cred = c
			case "other":
				other = c
			}
		}

		assert.Empty(t, cred.Token)
		assert.Empty(t, cred.RefreshToken)
		assert.Equal(t, "token-b", other.Token)
		assert.Equal(t, "refresh-b", other.RefreshToken)
	})

	t.Run("returns error when no current context is set", func(t *testing.T) {
		testutil.SetupTestHome(t)
		// No config file — no current context

		err := NewLogoutImpl().Logout()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current context")
	})

	t.Run("returns error when context has no associated credential", func(t *testing.T) {
		home := testutil.SetupTestHome(t)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://localhost"}},
			Credentials:    []config.Credential{},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp"}},
		})

		err := NewLogoutImpl().Logout()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current credential")
	})
}
