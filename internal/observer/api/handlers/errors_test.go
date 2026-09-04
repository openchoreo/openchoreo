// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// TestRequireJSONContentType covers the Content-Type check, which is the only
// thing standing between a non-JSON body and the strict layer's unconditional
// decode.
func TestRequireJSONContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"json is accepted", "application/json", http.StatusOK},
		{"json with charset is accepted", "application/json; charset=utf-8", http.StatusOK},
		{"absent is accepted", "", http.StatusOK},
		{"form encoding is rejected", "application/x-www-form-urlencoded", http.StatusBadRequest},
		{"text is rejected", "text/plain", http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// A body that would otherwise validate, so a 400 can only come from
			// the Content-Type check.
			body := `{"searchScope":{"namespace":"ns"},` +
				`"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-02T00:00:00Z"}`

			svc := servicemocks.NewMockLogsQuerier(t)
			svc.On("QueryLogs", mock.Anything, mock.Anything).
				Return(&types.LogsQueryResponse{}, nil).Maybe()

			h := &Handler{
				baseHandler: baseHandler{logger: noopLogger()},
				logsService: svc,
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", strings.NewReader(body))
			// httptest.NewRequest defaults no Content-Type; set only when the
			// case calls for one.
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			} else {
				req.Header.Del("Content-Type")
			}

			rr := serve(t, h, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}
}
