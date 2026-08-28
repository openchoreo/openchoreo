// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// TestOptionalTriggerBodyMiddleware covers the empty-body substitution that keeps a bodyless
// `POST .../trigger` working.
//
// oapi-codegen's strict handler decodes the request body unconditionally once an operation
// declares a requestBody, so without this middleware an empty body fails with
// "can't decode JSON body: EOF" and the endpoint's original bodyless form returns 400.
func TestOptionalTriggerBodyMiddleware(t *testing.T) {
	const triggerPath = "/api/v1alpha1/namespaces/ns-1/releasebindings/rb-1/trigger"

	// decodeSpy stands in for the generated strict handler: it decodes the body the same way and
	// reports what it saw.
	newSpy := func(seen *gen.CronJobTriggerRequest, decodeErr *error) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body gen.CronJobTriggerRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				*decodeErr = err
				return
			}
			*seen = body
		})
	}

	cases := []struct {
		name      string
		body      io.Reader
		wantArgs  *[]string
		wantNoErr bool
	}{
		{name: "no body decodes as empty object", body: nil, wantNoErr: true},
		{name: "empty string body decodes as empty object", body: strings.NewReader(""), wantNoErr: true},
		{name: "whitespace-only body decodes as empty object", body: strings.NewReader("  \n"), wantNoErr: true},
		{name: "explicit empty object passes through", body: strings.NewReader(`{}`), wantNoErr: true},
		{
			name:      "args pass through unchanged",
			body:      strings.NewReader(`{"args":["--mode","backfill"]}`),
			wantArgs:  &[]string{"--mode", "backfill"},
			wantNoErr: true,
		},
		{
			name:      "empty args array survives as non-nil",
			body:      strings.NewReader(`{"args":[]}`),
			wantArgs:  &[]string{},
			wantNoErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen gen.CronJobTriggerRequest
			var decodeErr error
			h := OptionalTriggerBodyMiddleware(newSpy(&seen, &decodeErr))

			req := httptest.NewRequest(http.MethodPost, triggerPath, tc.body)
			h.ServeHTTP(httptest.NewRecorder(), req)

			if tc.wantNoErr {
				require.NoError(t, decodeErr, "body should decode without error")
			}
			if tc.wantArgs == nil {
				require.Nil(t, seen.Args, "args should be absent")
				return
			}
			require.NotNil(t, seen.Args, "args should be present and non-nil")
			require.Equal(t, *tc.wantArgs, *seen.Args)
		})
	}
}

// TestOptionalTriggerBodyMiddlewareScope confirms the middleware only rewrites trigger requests.
func TestOptionalTriggerBodyMiddlewareScope(t *testing.T) {
	// A path that merely ends in /trigger is not the releasebinding subresource and must be
	// left alone, so a future endpoint cannot silently inherit this behavior.
	t.Run("leaves lookalike /trigger paths alone", func(t *testing.T) {
		var gotBody string
		h := OptionalTriggerBodyMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
		}))

		req := httptest.NewRequest(http.MethodPost,
			"/api/v1alpha1/namespaces/ns-1/workflows/wf-1/trigger", strings.NewReader(""))
		h.ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, gotBody, "only the releasebinding trigger subresource should be rewritten")
	})

	t.Run("leaves non-trigger paths alone", func(t *testing.T) {
		var gotBody string
		h := OptionalTriggerBodyMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/namespaces/ns-1/components", strings.NewReader(""))
		h.ServeHTTP(httptest.NewRecorder(), req)

		// An empty body stays empty so other endpoints keep their own validation behavior.
		require.Empty(t, gotBody)
	})

	t.Run("leaves non-POST methods alone", func(t *testing.T) {
		var gotBody string
		h := OptionalTriggerBodyMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
		}))

		req := httptest.NewRequest(http.MethodGet,
			"/api/v1alpha1/namespaces/ns-1/releasebindings/rb-1/trigger", strings.NewReader(""))
		h.ServeHTTP(httptest.NewRecorder(), req)

		require.Empty(t, gotBody)
	})
}

// TestOptionalTriggerBodyMiddlewareSizeLimit covers the pre-auth read cap.
//
// The middleware runs before authentication, so an unauthenticated caller must not be able to
// make it buffer an unbounded body.
func TestOptionalTriggerBodyMiddlewareSizeLimit(t *testing.T) {
	const triggerPath = "/api/v1alpha1/namespaces/ns-1/releasebindings/rb-1/trigger"

	t.Run("oversized body is rejected with 413", func(t *testing.T) {
		reached := false
		h := OptionalTriggerBodyMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))

		oversized := strings.Repeat("a", maxTriggerPayloadSize+1)
		req := httptest.NewRequest(http.MethodPost, triggerPath, strings.NewReader(oversized))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		require.False(t, reached, "handler must not run for an oversized body")
	})

	t.Run("body at the limit is accepted", func(t *testing.T) {
		var seen gen.CronJobTriggerRequest
		h := OptionalTriggerBodyMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&seen)
		}))

		// A valid args payload padded to exactly the cap.
		prefix := `{"args":["`
		suffix := `"]}`
		pad := strings.Repeat("x", maxTriggerPayloadSize-len(prefix)-len(suffix))
		req := httptest.NewRequest(http.MethodPost, triggerPath, strings.NewReader(prefix+pad+suffix))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		require.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code)
		require.NotNil(t, seen.Args)
		require.Len(t, *seen.Args, 1)
	})
}
