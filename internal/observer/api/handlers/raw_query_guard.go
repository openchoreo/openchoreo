// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/httputil"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// RawTracesQueryPath is the route of the raw trace query operation.
const RawTracesQueryPath = "/api/v1alpha1/traces/raw-query"

// DefaultRawQueryMaxRequestBytes caps a raw query body when none is configured.
const DefaultRawQueryMaxRequestBytes int64 = 1 << 20

func isRawTracesQuery(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == RawTracesQueryPath
}

// rawTracesQueryGuard checks the body of a raw trace query before the
// generated layer decodes it. The body is the tracing backend's native
// request, so it may be JSON or text: any other media type gets 415, and a
// body over maxBytes gets 413. Other routes pass through untouched.
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

			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || (mediaType != "application/json" && mediaType != "text/plain") {
				writeRawQueryError(logger, w, http.StatusUnsupportedMediaType, gen.UnsupportedMediaType,
					types.ErrorCodeV1TracesRawQueryMediaType,
					"Send the tracing backend's native request as application/json or text/plain")
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
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
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			next.ServeHTTP(w, r)
		})
	}
}

func writeRawQueryError(
	logger *slog.Logger, w http.ResponseWriter, status int, title gen.ErrorResponseTitle, code, msg string,
) {
	if err := httputil.WriteJSON(w, status, errorPayload(title, code, msg)); err != nil {
		logger.Error("Failed to write raw query error response", "error", err)
	}
}
