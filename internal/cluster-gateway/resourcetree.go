// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

const (
	// resourceTreePathPrefix is the mux pattern this endpoint is registered on.
	// It comes from the protocol package so the route the gateway serves cannot
	// drift from the one the control plane builds.
	resourceTreePathPrefix = protocol.GatewayPathPrefix

	// maxResourceTreeRequestBytes caps the match request body. A batch is at
	// most protocol.MaxQueriesPerRequest queries of protocol.MaxParentsPerQuery
	// parent refs, so a legitimate request stays far below this; the cap exists
	// to stop an unbounded frame from being pushed through the WebSocket.
	maxResourceTreeRequestBytes int64 = 10 << 20 // 10MB

	// resourceTreeTunnelTimeout is the middle rung of protocol's timeout ladder,
	// which owns the ordering against the agent's per-batch budget and the
	// caller's client timeout.
	resourceTreeTunnelTimeout = protocol.GatewayTunnelTimeout
)

// sendResourceTreeMatchRequest defers to the server's CR-authorized tunnel send.
// It is a package-level var so tests can exercise the handler without a live
// agent connection, mirroring getAgentConnectionForWirelogs.
var sendResourceTreeMatchRequest = func(
	s *Server, planeIdentifier, crKey string,
	req *messaging.HTTPTunnelRequest, timeout time.Duration,
) (*messaging.HTTPTunnelResponse, error) {
	return s.SendHTTPTunnelRequestForCR(planeIdentifier, crKey, req, timeout)
}

// handleResourceTreeMatches forwards a resource tree match batch to the cluster
// agent and returns the agent's answer verbatim.
// URL: POST /api/resource-tree/{planeType}/{planeID}/{crNamespace}/{crName}/matches
// Body: a protocol.MatchRequest JSON document.
//
// This endpoint deliberately never answers 404 of its own accord. The control
// plane reads a 404 from this path as "the agent predates resource tree
// support" and falls back to its legacy traversal, so a routing or
// authorization failure raised here must surface as 403/502 instead — a 404
// would silently downgrade a real failure into the slow path.
func (s *Server) handleResourceTreeMatches(w http.ResponseWriter, r *http.Request) {
	requestID := getOrGenerateRequestID(r)
	logger := s.logger.With("requestId", requestID)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed: resource-tree matches accepts POST only", http.StatusMethodNotAllowed)
		return
	}

	planeType, planeID, crNamespace, crName, ok := protocol.ParseGatewayMatchesPath(r.URL.Path)
	if !ok {
		logger.Warn("invalid resource-tree URL format", "path", r.URL.Path)
		http.Error(w,
			"invalid resource-tree URL: expected /api/resource-tree/{planeType}/{planeID}/{crNamespace}/{crName}/matches",
			http.StatusBadRequest)
		return
	}

	body, err := readCappedBody(w, r, maxResourceTreeRequestBytes)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			logger.Warn("resource-tree request body too large", "limit", maxResourceTreeRequestBytes)
			http.Error(w, fmt.Sprintf("request body exceeds the %d byte limit", maxResourceTreeRequestBytes),
				http.StatusRequestEntityTooLarge)
			return
		}
		logger.Warn("failed to read resource-tree request body", "error", err)
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	planeIdentifier := fmt.Sprintf("%s/%s", planeType, planeID)
	// Cluster-scoped CRs carry the crNamespaceClusterPlaceholder sentinel so the
	// key matches the "/name" form the connection manager registers.
	if crNamespace == crNamespaceClusterPlaceholder {
		crNamespace = ""
	}
	crKey := fmt.Sprintf("%s/%s", crNamespace, crName)

	logger.Info("resource-tree match request received",
		"plane", planeIdentifier,
		"cr", crKey,
		"bodyBytes", len(body),
	)

	tunnelReq := messaging.NewHTTPTunnelRequest(
		protocol.Target,
		http.MethodPost,
		protocol.PathMatches,
		"",
		map[string][]string{"Content-Type": {"application/json"}},
		body,
	)
	tunnelReq.GatewayRequestID = requestID

	response, err := sendResourceTreeMatchRequest(s, planeIdentifier, crKey, tunnelReq, resourceTreeTunnelTimeout)
	if err != nil {
		if strings.Contains(err.Error(), "no agents authorized for CR") {
			logger.Warn("CR authorization failed", "plane", planeIdentifier, "cr", crKey, "error", err)
			http.Error(w, fmt.Sprintf("Forbidden: Agent not authorized for CR %s", crKey), http.StatusForbidden)
			return
		}
		logger.Error("resource-tree tunnel request failed", "plane", planeIdentifier, "cr", crKey, "error", err)
		http.Error(w, fmt.Sprintf("resource-tree request failed: %v", err), http.StatusBadGateway)
		return
	}

	// An agent that has no resource-tree target replies through the ordinary
	// response channel with Error set and an EMPTY Body
	// (messaging.NewHTTPTunnelErrorResponse), so the message has to be
	// materialized here or the version-skew signal never leaves the gateway.
	if response.Error != nil {
		status := sanitizeTunnelStatus(response.StatusCode)
		logger.Warn("agent reported an error for resource-tree match",
			"plane", planeIdentifier,
			"cr", crKey,
			"statusCode", status,
			"message", response.Error.Message,
		)
		http.Error(w, response.Error.Message, status)
		return
	}

	if contentType := headerValue(response.Headers, "Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(sanitizeTunnelStatus(response.StatusCode))
	if len(response.Body) > 0 {
		if _, err := w.Write(response.Body); err != nil {
			logger.Warn("failed to write resource-tree response body", "error", err)
		}
	}

	logger.Info("resource-tree match request completed",
		"plane", planeIdentifier,
		"cr", crKey,
		"statusCode", response.StatusCode,
	)
}

// readCappedBody reads the request body, refusing anything past limit. The
// ResponseWriter is handed to MaxBytesReader so the server also stops reading
// the rest of an oversized upload instead of draining it.
func readCappedBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
}

// sanitizeTunnelStatus keeps a malformed agent status code from reaching
// WriteHeader, which panics outside the valid range. An unusable code is
// reported as a bad gateway — never as 404, which the caller reads as version
// skew.
func sanitizeTunnelStatus(statusCode int) int {
	if statusCode < 100 || statusCode > 599 {
		return http.StatusBadGateway
	}
	return statusCode
}

// headerValue reads the first value of a tunnel response header,
// case-insensitively: the agent's map is plain JSON, not an http.Header, so its
// keys arrive in whatever casing the data plane produced.
func headerValue(headers map[string][]string, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
