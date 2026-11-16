// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func TestExportCommand_FetchesAndPrintsCount(t *testing.T) {
	f := newFetchFixture(t, false)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), fetchContextKey{}, "command"))
	defer cancel()
	f.override = func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "command", req.Context().Value(fetchContextKey{}))
		return nil, nil
	}
	cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
	cmd.SetContext(ctx)
	var output bytes.Buffer
	cmd.SetOut(&output)
	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Equal(t, "Fetched 58 resources.\n", output.String())
}

func TestExportCommand_ClientCreationError(t *testing.T) {
	wantErr := errors.New("no control plane configured")
	cmd := NewExportCmd(func() (client.Interface, error) { return nil, wantErr })
	var output bytes.Buffer
	cmd.SetOut(&output)
	require.ErrorIs(t, cmd.RunE(cmd, nil), wantErr)
	assert.Empty(t, output.String())
}

func TestExportCommand_FetchError(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(_ *http.Request) (*http.Response, error) {
		return testutil.JSONResp(http.StatusForbidden, map[string]string{"error": "access denied"}), nil
	}
	cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
	var output bytes.Buffer
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)
	require.ErrorContains(t, cmd.RunE(cmd, nil), "access denied")
	assert.Empty(t, output.String(), "a failed fetch must not print a success count")
}

func TestExportCommand_RequiresLogin(t *testing.T) {
	testutil.SetupTestHome(t)
	called := false
	cmd := NewExportCmd(func() (client.Interface, error) {
		called = true
		return nil, errors.New("must not create client before login")
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})
	require.ErrorContains(t, cmd.Execute(), "Authentication required")
	assert.False(t, called)
}

func TestExportCommand_RejectsArguments(t *testing.T) {
	called := false
	cmd := NewExportCmd(func() (client.Interface, error) {
		called = true
		return nil, errors.New("unexpected client creation")
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"unexpected"})
	require.Error(t, cmd.Execute())
	assert.False(t, called)
}
