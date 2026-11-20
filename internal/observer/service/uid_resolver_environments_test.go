// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListEnvironmentsFollowsTheCursor pins that every page is read and that
// isProduction comes through. Reading only the first page would leave a
// production environment on a later one unknown, and the DORA tools would
// default to combining environments instead.
func TestListEnvironmentsFollowsTheCursor(t *testing.T) {
	t.Parallel()

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()

	var cursors []string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/namespaces/acme/environments", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("limit"))
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		w.Header().Set("Content-Type", "application/json")
		if cursor == "" {
			_, _ = w.Write([]byte(`{"items":[
				{"metadata":{"name":"development"},"spec":{}},
				{"metadata":{"name":"staging"},"spec":{"isProduction":false}}
			],"pagination":{"nextCursor":"page-2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[
			{"metadata":{"name":"production"},"spec":{"isProduction":true}}
		],"pagination":{}}`))
	}))
	defer apiSrv.Close()

	envs, err := newTestResolver(t, apiSrv, tokenSrv, nil).ListEnvironments(context.Background(), "acme")
	require.NoError(t, err)
	assert.Equal(t, []Environment{
		{Name: "development"},
		{Name: "staging"},
		{Name: "production", IsProduction: true},
	}, envs)
	assert.Equal(t, []string{"", "page-2"}, cursors)
}

func TestListEnvironmentsReportsAnUnknownNamespace(t *testing.T) {
	t.Parallel()

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiSrv.Close()

	_, err := newTestResolver(t, apiSrv, tokenSrv, nil).ListEnvironments(context.Background(), "nope")
	require.Error(t, err)
	var notFound *NotFoundError
	assert.True(t, errors.As(err, &notFound), "a missing namespace should be a NotFoundError, got %v", err)
}
