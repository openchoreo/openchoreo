// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type mockImpl struct {
	api.CommandImplementationInterface
}

func (m *mockImpl) Logout() error { return nil }

func TestNewLogoutCmd_Structure(t *testing.T) {
	cmd := NewLogoutCmd(&mockImpl{})

	assert.Equal(t, "logout", cmd.Use)
	assert.Empty(t, cmd.Commands(), "logout should have no subcommands")
}
