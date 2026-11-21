// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

// TestListEnvironmentsRefusesAnIncompleteList pins that paging which cannot
// finish is an error, not a short or repeated list. A repeated cursor listed the
// same environments again, so one production environment read as several; a
// page cap left the rest unread, so a production environment on them went
// unnoticed. Either way the DORA tools would have chosen wrongly.
func TestListEnvironmentsRefusesAnIncompleteList(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		cursor func(page int) string
		want   string
	}{
		{"a cursor that does not advance", func(int) string { return "same" }, `returned cursor "same" again`},
		{"a cursor that comes back later", func(page int) string { return []string{"a", "b", "a"}[page%3] },
			`returned cursor "a" again`},
		{"more pages than the cap", func(page int) string { return fmt.Sprintf("page-%d", page+1) },
			fmt.Sprintf("more than %d pages", environmentListMaxPages)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tokenSrv := newAlwaysOKTokenServer(t)
			defer tokenSrv.Close()
			var page int32
			apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				n := int(atomic.AddInt32(&page, 1)) - 1
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"items":[{"metadata":{"name":"env-%d"},"spec":{"isProduction":true}}],`+
					`"pagination":{"nextCursor":%q}}`, n, tc.cursor(n))
			}))
			defer apiSrv.Close()

			envs, err := newTestResolver(t, apiSrv, tokenSrv, nil).ListEnvironments(context.Background(), "acme")
			require.Error(t, err)
			assert.Nil(t, envs)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
