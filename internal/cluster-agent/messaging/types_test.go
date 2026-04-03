// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messaging

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMessageID(t *testing.T) {
	id1 := GenerateMessageID()
	id2 := GenerateMessageID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "generated IDs should be unique")
}

func TestNewHTTPTunnelRequest(t *testing.T) {
	headers := map[string][]string{"Content-Type": {"application/json"}}
	body := []byte(`{"key":"value"}`)

	req := NewHTTPTunnelRequest("k8s", "GET", "/api/v1/pods", "namespace=default", headers, body)

	assert.Equal(t, "k8s", req.Target)
	assert.Equal(t, "GET", req.Method)
	assert.Equal(t, "/api/v1/pods", req.Path)
	assert.Equal(t, "namespace=default", req.Query)
	assert.Equal(t, headers, req.Headers)
	assert.Equal(t, body, req.Body)
	assert.Empty(t, req.RequestID, "RequestID should not be set by factory")
}

func TestNewHTTPTunnelRequest_NilFields(t *testing.T) {
	req := NewHTTPTunnelRequest("k8s", "GET", "/api/v1/pods", "", nil, nil)

	assert.Nil(t, req.Headers)
	assert.Nil(t, req.Body)
	assert.Empty(t, req.Query)
}

func TestNewHTTPTunnelSuccessResponse(t *testing.T) {
	req := &HTTPTunnelRequest{RequestID: "req-123"}
	headers := map[string][]string{"Content-Type": {"application/json"}}
	body := []byte(`{"items":[]}`)

	resp := NewHTTPTunnelSuccessResponse(req, http.StatusOK, headers, body)

	assert.Equal(t, "req-123", resp.RequestID)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, headers, resp.Headers)
	assert.Equal(t, body, resp.Body)
	assert.Nil(t, resp.Error)
}

func TestNewHTTPTunnelErrorResponse(t *testing.T) {
	req := &HTTPTunnelRequest{RequestID: "req-456"}

	resp := NewHTTPTunnelErrorResponse(req, http.StatusNotFound, "resource not found")

	assert.Equal(t, "req-456", resp.RequestID)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Equal(t, http.StatusNotFound, resp.Error.Code)
	assert.Equal(t, "resource not found", resp.Error.Message)
	assert.Nil(t, resp.Body)
	assert.Nil(t, resp.Headers)
}

func TestHTTPTunnelResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{"200 OK", http.StatusOK, true},
		{"201 Created", http.StatusCreated, true},
		{"204 No Content", http.StatusNoContent, true},
		{"299 boundary", 299, true},
		{"300 redirect", 300, false},
		{"400 Bad Request", http.StatusBadRequest, false},
		{"404 Not Found", http.StatusNotFound, false},
		{"500 Internal", http.StatusInternalServerError, false},
		{"199 below range", 199, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &HTTPTunnelResponse{StatusCode: tt.statusCode}
			assert.Equal(t, tt.expected, resp.IsSuccess())
		})
	}
}

func TestHTTPTunnelResponse_IsError(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		resp := &HTTPTunnelResponse{
			Error: &ErrorDetails{Message: "something went wrong"},
		}
		assert.True(t, resp.IsError())
	})

	t.Run("without error", func(t *testing.T) {
		resp := &HTTPTunnelResponse{StatusCode: http.StatusOK}
		assert.False(t, resp.IsError())
	})
}

func TestNewHTTPTunnelStreamInit(t *testing.T) {
	headers := map[string][]string{"Connection": {"Upgrade"}}

	init := NewHTTPTunnelStreamInit("k8s", "GET", "/api/v1/pods", "watch=true", headers, true, "SPDY/3.1")

	assert.Equal(t, "k8s", init.Target)
	assert.Equal(t, "GET", init.Method)
	assert.Equal(t, "/api/v1/pods", init.Path)
	assert.Equal(t, "watch=true", init.Query)
	assert.Equal(t, headers, init.Headers)
	assert.True(t, init.IsUpgrade)
	assert.Equal(t, "SPDY/3.1", init.UpgradeProto)
}

func TestNewHTTPTunnelStreamInit_NoUpgrade(t *testing.T) {
	init := NewHTTPTunnelStreamInit("k8s", "GET", "/api/v1/pods", "watch=true", nil, false, "")

	assert.False(t, init.IsUpgrade)
	assert.Empty(t, init.UpgradeProto)
}

func TestNewHTTPTunnelStreamChunk(t *testing.T) {
	data := []byte("stream data chunk")

	chunk := NewHTTPTunnelStreamChunk("req-789", data, false)

	assert.Equal(t, "req-789", chunk.RequestID)
	assert.Equal(t, data, chunk.Data)
	assert.False(t, chunk.IsClose)
}

func TestNewHTTPTunnelStreamChunk_Close(t *testing.T) {
	chunk := NewHTTPTunnelStreamChunk("req-789", nil, true)

	assert.True(t, chunk.IsClose)
	assert.Nil(t, chunk.Data)
}

func TestNewHTTPTunnelStreamResponse(t *testing.T) {
	init := &HTTPTunnelStreamInit{RequestID: "stream-123"}
	headers := map[string][]string{"Content-Type": {"application/json"}}

	resp := NewHTTPTunnelStreamResponse(init, http.StatusOK, headers)

	assert.Equal(t, "stream-123", resp.RequestID)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, headers, resp.Headers)
	assert.Nil(t, resp.Error)
}

func TestNewHTTPTunnelStreamErrorResponse(t *testing.T) {
	init := &HTTPTunnelStreamInit{RequestID: "stream-456"}

	resp := NewHTTPTunnelStreamErrorResponse(init, http.StatusBadGateway, "backend failed")

	assert.Equal(t, "stream-456", resp.RequestID)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.NotNil(t, resp.Error)
	assert.Equal(t, http.StatusBadGateway, resp.Error.Code)
	assert.Equal(t, "backend failed", resp.Error.Message)
}
