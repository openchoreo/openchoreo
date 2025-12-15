// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package messaging

import (
	"net/http"

	"github.com/google/uuid"
)

// GenerateMessageID generates a unique message ID
func GenerateMessageID() string {
	return uuid.New().String()
}

// ErrorDetails provides structured error information
type ErrorDetails struct {
	Code    int            `json:"code,omitempty"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// HTTPTunnelRequest represents an HTTP request to be proxied through the cluster agent
// This enables proxying raw HTTP requests to Kubernetes API and other services in the data plane
type HTTPTunnelRequest struct {
	// RequestID is a unique identifier for this request (UUID)
	// Used for matching request-response pairs in the WebSocket layer
	RequestID string `json:"requestID"`
	// GatewayRequestID is the request ID from the original HTTP request at the gateway
	// Used for end-to-end request tracing and correlation
	GatewayRequestID string `json:"gatewayRequestID,omitempty"`
	// Target identifies the backend service ("k8s", "monitoring", "logs", etc.)
	Target  string              `json:"target"`
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Query   string              `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// HTTPTunnelResponse represents an HTTP response from the data plane backend service
type HTTPTunnelResponse struct {
	RequestID  string              `json:"requestID"`
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       []byte              `json:"body,omitempty"`
	Error      *ErrorDetails       `json:"error,omitempty"`
}

func NewHTTPTunnelRequest(target, method, path, query string, headers map[string][]string, body []byte) *HTTPTunnelRequest {
	return &HTTPTunnelRequest{
		Target:  target,
		Method:  method,
		Path:    path,
		Query:   query,
		Headers: headers,
		Body:    body,
	}
}

func NewHTTPTunnelSuccessResponse(req *HTTPTunnelRequest, statusCode int, headers map[string][]string, body []byte) *HTTPTunnelResponse {
	return &HTTPTunnelResponse{
		RequestID:  req.RequestID,
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}
}

func NewHTTPTunnelErrorResponse(req *HTTPTunnelRequest, statusCode int, errMsg string) *HTTPTunnelResponse {
	return &HTTPTunnelResponse{
		RequestID:  req.RequestID,
		StatusCode: statusCode,
		Error: &ErrorDetails{
			Code:    statusCode,
			Message: errMsg,
		},
	}
}

func (r *HTTPTunnelResponse) IsSuccess() bool {
	return r.StatusCode >= http.StatusOK && r.StatusCode < http.StatusMultipleChoices
}

func (r *HTTPTunnelResponse) IsError() bool {
	return r.Error != nil
}

type HTTPTunnelStreamInit struct {
	RequestID    string              `json:"requestID"`
	Target       string              `json:"target"`
	Method       string              `json:"method"`
	Path         string              `json:"path"`
	Query        string              `json:"query,omitempty"`
	Headers      map[string][]string `json:"headers,omitempty"`
	IsUpgrade    bool                `json:"isUpgrade"`              // True for SPDY/WebSocket upgrades
	UpgradeProto string              `json:"upgradeProto,omitempty"` // "SPDY/3.1", "websocket", etc.
}

type HTTPTunnelStreamChunk struct {
	RequestID string `json:"requestID"`
	Data      []byte `json:"data"`
	StreamID  int    `json:"streamId,omitempty"` // For multiplexed streams (SPDY)
	IsClose   bool   `json:"isClose,omitempty"`  // True when stream ends
}

type HTTPTunnelStreamResponse struct {
	RequestID  string              `json:"requestID"`
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Error      *ErrorDetails       `json:"error,omitempty"`
}

func NewHTTPTunnelStreamInit(target, method, path, query string, headers map[string][]string, isUpgrade bool, upgradeProto string) *HTTPTunnelStreamInit {
	return &HTTPTunnelStreamInit{
		Target:       target,
		Method:       method,
		Path:         path,
		Query:        query,
		Headers:      headers,
		IsUpgrade:    isUpgrade,
		UpgradeProto: upgradeProto,
	}
}

func NewHTTPTunnelStreamChunk(requestID string, data []byte, isClose bool) *HTTPTunnelStreamChunk {
	return &HTTPTunnelStreamChunk{
		RequestID: requestID,
		Data:      data,
		IsClose:   isClose,
	}
}

func NewHTTPTunnelStreamResponse(req *HTTPTunnelStreamInit, statusCode int, headers map[string][]string) *HTTPTunnelStreamResponse {
	return &HTTPTunnelStreamResponse{
		RequestID:  req.RequestID,
		StatusCode: statusCode,
		Headers:    headers,
	}
}

func NewHTTPTunnelStreamErrorResponse(req *HTTPTunnelStreamInit, statusCode int, errMsg string) *HTTPTunnelStreamResponse {
	return &HTTPTunnelStreamResponse{
		RequestID:  req.RequestID,
		StatusCode: statusCode,
		Error: &ErrorDetails{
			Code:    statusCode,
			Message: errMsg,
		},
	}
}
