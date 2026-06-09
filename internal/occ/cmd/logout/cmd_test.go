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

func TestNewLogoutCmd_Structure(t *testing.T) {
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
