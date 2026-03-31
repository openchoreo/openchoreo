// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type mockLoginAPI struct {
	loggedIn    bool
	loginPrompt string
}

func (m *mockLoginAPI) Login(_ api.LoginParams) error { return nil }
func (m *mockLoginAPI) IsLoggedIn() bool              { return m.loggedIn }
func (m *mockLoginAPI) GetLoginPrompt() string        { return m.loginPrompt }

func TestRequireLogin(t *testing.T) {
	tests := []struct {
		name        string
		loggedIn    bool
		loginPrompt string
		wantErr     bool
		errMsg      string
	}{
		{
			name:     "logged in - no error",
			loggedIn: true,
			wantErr:  false,
		},
		{
			name:        "not logged in - returns error with prompt",
			loggedIn:    false,
			loginPrompt: "Please login first using 'occ login'",
			wantErr:     true,
			errMsg:      "Please login first using 'occ login'",
		},
		{
			name:        "not logged in - custom prompt",
			loggedIn:    false,
			loginPrompt: "Authentication required",
			wantErr:     true,
			errMsg:      "Authentication required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl := &mockLoginAPI{loggedIn: tt.loggedIn, loginPrompt: tt.loginPrompt}
			handler := RequireLogin(impl)

			cmd := &cobra.Command{Use: "get"}
			err := handler(cmd, []string{})

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
