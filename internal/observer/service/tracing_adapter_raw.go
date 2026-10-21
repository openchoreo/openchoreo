// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

var (
	// ErrRawTracesNotSupported is returned when the tracing adapter answers a
	// raw query with 501.
	ErrRawTracesNotSupported = errors.New("raw trace queries are not supported by the tracing backend")
	// ErrRawTracesTimeout is returned when a raw query runs past its timeout.
	ErrRawTracesTimeout = errors.New("raw trace query timed out")
	// ErrRawTracesResultTooLarge is returned when the adapter's response is
	// larger than the maximum response size.
	ErrRawTracesResultTooLarge = errors.New("raw trace query result exceeds the maximum response size")
	// ErrRawTracesRejected is returned when the tracing adapter rejects a raw
	// query. The concrete error is a *RawTracesRejectedError carrying its reason.
	ErrRawTracesRejected = errors.New("raw trace query rejected by the tracing backend")
	// ErrRawTracesInvalidRequest is returned for a raw query request the
	// Observer refuses before forwarding it.
	ErrRawTracesInvalidRequest = errors.New("invalid raw trace query request")
	// ErrRawTracesRequestTooLarge is returned when the tracing adapter answers
	// 413: the request is larger than it accepts.
	ErrRawTracesRequestTooLarge = errors.New("raw trace query request is too large for the tracing backend")
	// ErrRawTracesUnsupportedMediaType is returned when the tracing adapter
	// answers 415: its backend doesn't take requests in that media type.
	ErrRawTracesUnsupportedMediaType = errors.New("the tracing backend does not accept raw queries in this media type")
)

// RawTracesRejectedError carries the tracing adapter's reason for rejecting a
// raw query, which names the query language the adapter expects.
type RawTracesRejectedError struct {
	Detail string
}

func (e *RawTracesRejectedError) Error() string {
	return ErrRawTracesRejected.Error() + ": " + e.Detail
}

func (e *RawTracesRejectedError) Unwrap() error { return ErrRawTracesRejected }

// maxRejectionDetailLength caps the adapter reason passed back to callers.
const maxRejectionDetailLength = 1024

// QueryRawTraces implements observability.TracingAdapter.
//
// The request body is forwarded with its media type, and the backend's
// response comes back unchanged. The adapter enforces the query timeout and
// the maximum result size against the backend. The Observer's client gives
// the adapter a margin over the timeout, and reads at most the maximum result
// size, so a misbehaving adapter cannot hold a request open or exhaust memory.
func (t *TracingAdapter) QueryRawTraces(
	ctx context.Context,
	params observability.RawTracesQueryParams,
) (*observability.RawTracesQueryResult, error) {
	resp, err := t.rawClient.QueryRawTracesWithBody(ctx, params.ContentType, bytes.NewReader(params.Body))
	if err != nil {
		if isTimeout(err) {
			return nil, ErrRawTracesTimeout
		}
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, t.rawMaxResponseBytes+1))
	if err != nil {
		if isTimeout(err) {
			return nil, ErrRawTracesTimeout
		}
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > t.rawMaxResponseBytes {
		return nil, ErrRawTracesResultTooLarge
	}

	switch resp.StatusCode {
	case http.StatusOK:
		if !json.Valid(body) {
			return nil, fmt.Errorf("the tracing adapter returned a response that is not JSON")
		}
		return &observability.RawTracesQueryResult{Body: body}, nil
	case http.StatusBadRequest:
		return nil, &RawTracesRejectedError{Detail: adapterErrorDetail(body)}
	case http.StatusRequestEntityTooLarge:
		return nil, ErrRawTracesRequestTooLarge
	case http.StatusUnsupportedMediaType:
		return nil, ErrRawTracesUnsupportedMediaType
	case http.StatusNotImplemented, http.StatusNotFound, http.StatusMethodNotAllowed:
		// A tracing module released before raw queries existed has no such
		// route: it answers 404 or 405, which means the same as 501 here.
		return nil, ErrRawTracesNotSupported
	case http.StatusGatewayTimeout:
		return nil, ErrRawTracesTimeout
	default:
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, truncate(string(body), maxRejectionDetailLength))
	}
}

// adapterErrorDetail extracts the reason from a tracing adapter error body,
// falling back to the raw body.
func adapterErrorDetail(body []byte) string {
	var e struct {
		Detail  string `json:"detail"`
		Message string `json:"message"`
	}
	detail := ""
	if json.Unmarshal(body, &e) == nil {
		detail = e.Detail
		if detail == "" {
			detail = e.Message
		}
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if detail == "" {
		detail = "the tracing backend rejected the query"
	}
	return truncate(detail, maxRejectionDetailLength)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
