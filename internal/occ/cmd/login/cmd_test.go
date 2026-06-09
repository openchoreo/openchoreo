// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func TestNewLoginCmd_Structure(t *testing.T) {
	cmd := NewLoginCmd()

	assert.Equal(t, "login", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewLoginCmd_Flags(t *testing.T) {
	cmd := NewLoginCmd()

	for _, name := range []string{"client-credentials", "client-id", "client-secret", "credential"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "expected flag %q", name)
	}
}

func TestNewLoginCmd_RunE_NoConfig(t *testing.T) {
	testutil.SetupTestHome(t)

	cmd := NewLoginCmd()
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get current context")
}

func TestNewLoginCmd_RunE_ClientCredentials_MissingCreds(t *testing.T) {
	home := testutil.SetupTestHome(t)
	t.Setenv("OCC_CLIENT_ID", "")
	t.Setenv("OCC_CLIENT_SECRET", "")

	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://localhost"}},
		Credentials:    []config.Credential{{Name: "cred"}},
		Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	})

	cmd := NewLoginCmd()
	require.NoError(t, cmd.Flags().Set("client-credentials", "true"))
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client ID and client secret are required")
}
