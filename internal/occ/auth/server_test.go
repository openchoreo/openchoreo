// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callbackRequest builds a GET request to CallbackPath with the given query params.
func callbackRequest(t *testing.T, params url.Values) *http.Request {
	t.Helper()
	u := &url.URL{Path: CallbackPath, RawQuery: params.Encode()}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)
	return req
}

func TestCallbackHandler(t *testing.T) {
	t.Run("sends auth code and writes success HTML on valid callback", func(t *testing.T) {
		ch := make(chan AuthResult, 1)
		handler := callbackHandler("expected-state", ch)

		req := callbackRequest(t, url.Values{
			"code":  {"mycode"},
			"state": {"expected-state"},
		})
		w := httptest.NewRecorder()
		handler(w, req)

		result := <-ch
		require.NoError(t, result.Err)
		assert.Equal(t, "mycode", result.Code)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Successfully authenticated")
	})

	t.Run("sends error and writes error HTML on state mismatch", func(t *testing.T) {
		ch := make(chan AuthResult, 1)
		handler := callbackHandler("expected-state", ch)

		req := callbackRequest(t, url.Values{
			"code":  {"mycode"},
			"state": {"wrong-state"},
		})
		w := httptest.NewRecorder()
		handler(w, req)

		result := <-ch
		require.Error(t, result.Err)
		assert.Contains(t, result.Err.Error(), "state mismatch")
		assert.Contains(t, w.Body.String(), "Authentication Failed")
	})

	t.Run("sends error and writes error HTML when OAuth error param present", func(t *testing.T) {
		ch := make(chan AuthResult, 1)
		handler := callbackHandler("expected-state", ch)

		req := callbackRequest(t, url.Values{
			"error": {"access_denied"},
			"state": {"expected-state"},
		})
		w := httptest.NewRecorder()
		handler(w, req)

		result := <-ch
		require.Error(t, result.Err)
		assert.Contains(t, result.Err.Error(), "authentication failed")
		assert.Contains(t, result.Err.Error(), "access_denied")
		assert.Contains(t, w.Body.String(), "Authentication Failed")
	})

	t.Run("sends error and writes error HTML when code is missing", func(t *testing.T) {
		ch := make(chan AuthResult, 1)
		handler := callbackHandler("expected-state", ch)

		req := callbackRequest(t, url.Values{
			"state": {"expected-state"},
		})
		w := httptest.NewRecorder()
		handler(w, req)

		result := <-ch
		require.Error(t, result.Err)
		assert.Contains(t, result.Err.Error(), "no authorization code received")
		assert.Contains(t, w.Body.String(), "Authentication Failed")
	})
}
