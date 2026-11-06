// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// RawTracesQueryPath is the route of the raw trace query operation.
const RawTracesQueryPath = "/api/v1alpha1/traces/raw-query"

// The media types a raw trace query body may have.
const (
	jsonMediaType = "application/json"
	textMediaType = "text/plain"
)

// DefaultRawQueryMaxRequestBytes caps a raw query body when none is configured.
const DefaultRawQueryMaxRequestBytes int64 = 1 << 20

func isRawTracesQuery(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == RawTracesQueryPath
}

// rawTracesQueryGuard checks the body of a raw trace query before the
// generated layer decodes it. The body is the tracing backend's native
// request, so it may be JSON or text: any other media type or a charset other
// than UTF-8 gets 415, a body over maxBytes gets 413, and an empty body, a body
// that isn't UTF-8 or a JSON body that isn't exactly one JSON value gets 400.
// Other routes pass through untouched.
func rawTracesQueryGuard(logger *slog.Logger, maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = DefaultRawQueryMaxRequestBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isRawTracesQuery(r) {
				next.ServeHTTP(w, r)
				return
			}

			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || (mediaType != jsonMediaType && mediaType != textMediaType) {
				writeRawQueryError(logger, w, http.StatusUnsupportedMediaType, gen.UnsupportedMediaType,
					types.ErrorCodeV1TracesRawQueryMediaType,
					"Send the tracing backend's native request as application/json or text/plain")
				return
			}
			// The body is forwarded and audited as UTF-8, so no other charset is
			// accepted.
			if charset, ok := params["charset"]; ok && !strings.EqualFold(charset, "utf-8") {
				writeRawQueryError(logger, w, http.StatusUnsupportedMediaType, gen.UnsupportedMediaType,
					types.ErrorCodeV1TracesRawQueryMediaType, "Send the request body as UTF-8")
				return
			}
			// Media types are case-insensitive, but the generated layer picks the
			// body by an exact prefix match, so pass the canonical form on.
			r.Header.Set("Content-Type", mediaType)

			body, err := io.ReadAll(io.LimitReader(r.Body, readLimit(maxBytes)))
			if err != nil {
				writeRawQueryError(logger, w, http.StatusBadRequest, gen.BadRequest,
					types.ErrorCodeV1TracesInvalidRequest, "Failed to read the request body")
				return
			}
			if int64(len(body)) > maxBytes {
				writeRawQueryError(logger, w, http.StatusRequestEntityTooLarge, gen.PayloadTooLarge,
					types.ErrorCodeV1TracesRawQueryRequestTooLarge,
					"The request body is larger than the maximum request size")
				return
			}
			if msg := invalidRawQueryBody(mediaType, body); msg != "" {
				writeRawQueryError(logger, w, http.StatusBadRequest, gen.BadRequest,
					types.ErrorCodeV1TracesInvalidRequest, msg)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			next.ServeHTTP(w, r)
		})
	}
}

// invalidRawQueryBody says what is wrong with a raw query body, or "" if
// nothing is. A JSON body must be exactly one JSON value: the generated layer
// decodes only the first value, so anything after it would be dropped
// silently. Both kinds of body must be UTF-8.
func invalidRawQueryBody(mediaType string, body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return "the request body is empty"
	}
	if !utf8.Valid(body) {
		return "The request body is not valid UTF-8"
	}
	if mediaType == jsonMediaType && !json.Valid(body) {
		return "The request body must be a single valid JSON value"
	}
	return ""
}

// readLimit is how much to read to tell whether a body is over maxBytes: one
// byte more than the limit, unless that would overflow.
func readLimit(maxBytes int64) int64 {
	if maxBytes < math.MaxInt64 {
		return maxBytes + 1
	}
	return maxBytes
}

func writeRawQueryError(
	logger *slog.Logger, w http.ResponseWriter, status int, title gen.ErrorResponseTitle, code, msg string,
) {
	if err := httputil.WriteJSON(w, status, errorPayload(title, code, msg)); err != nil {
		logger.Error("Failed to write raw query error response", "error", err)
	}
}
