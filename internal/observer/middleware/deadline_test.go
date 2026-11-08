// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtendWriteDeadline(t *testing.T) {
	t.Parallel()

	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})
	handler := ExtendWriteDeadline(http.MethodPost, "/slow", 5*time.Second, slog.New(slog.DiscardHandler))(slow)

	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)

	t.Run("matching route gets the longer deadline", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Post(srv.URL+"/slow", "application/json", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "done", string(body))
	})

	t.Run("other routes keep the server deadline", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Post(srv.URL+"/other", "application/json", nil)
		if err == nil {
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body)
		}
		assert.Error(t, err, "the server write timeout should cut the response off")
	})
}
