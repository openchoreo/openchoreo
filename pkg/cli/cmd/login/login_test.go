// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type mockImpl struct{}

func (m *mockImpl) Login(_ api.LoginParams) error { return nil }
func (m *mockImpl) IsLoggedIn() bool              { return true }
func (m *mockImpl) GetLoginPrompt() string        { return "" }
func (m *mockImpl) Logout() error                 { return nil }

func TestNewLoginCmd_Flags(t *testing.T) {
	cmd := NewLoginCmd(&mockImpl{})

	assert.Equal(t, "login", cmd.Use)

	expectedFlags := []string{"client-credentials", "client-id", "client-secret", "credential"}
	for _, name := range expectedFlags {
		assert.NotNil(t, cmd.Flags().Lookup(name), "expected flag --%s", name)
	}
}
