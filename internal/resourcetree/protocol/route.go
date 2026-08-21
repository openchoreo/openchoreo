// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import "strings"

// The gateway leg of the route. PathMatches above is the tunnel leg — the path
// the gateway forwards to the agent — and this is the HTTP route the control
// plane calls the gateway on. Both legs belong to the wire contract, so both are
// spelled here exactly once: the caller builds the path with
// BuildGatewayMatchesPath and the gateway parses it with
// ParseGatewayMatchesPath, so a renamed segment cannot reach only one side.
//
// Drift here is silent rather than loud: a mismatched path 404s, and a 404 is
// the one status the client reads as "this agent predates resource-tree
// matching", which permanently downgrades discovery to the slow control-plane
// walk behind a single warn line.
const (
	// GatewayPathPrefix is also the mux pattern the gateway registers the
	// handler on, so registration and parsing cannot disagree either.
	GatewayPathPrefix = "/api/resource-tree/"

	// gatewayMatchesSegment is the fixed final path segment. Routing on it
	// rather than accepting any suffix keeps room for a second verb later
	// without the gateway silently forwarding a path the agent ignores.
	gatewayMatchesSegment = "matches"
)

// BuildGatewayMatchesPath renders the gateway route for one CR's match batch:
// /api/resource-tree/{planeType}/{planeID}/{crNamespace}/{crName}/matches. The
// result is the path only; callers prefix it with the gateway base URL.
//
// The segments are not escaped, matching what the gateway's parser accepts: a
// segment carrying a '/' would split into two and be rejected there rather than
// forwarded as something else.
func BuildGatewayMatchesPath(planeType, planeID, crNamespace, crName string) string {
	return GatewayPathPrefix +
		planeType + "/" + planeID + "/" + crNamespace + "/" + crName + "/" + gatewayMatchesSegment
}

// ParseGatewayMatchesPath splits a gateway route back into its four routing
// segments. Every segment must be non-empty and the path must end in the
// matches segment; anything else is a client error, never a forwarded request.
func ParseGatewayMatchesPath(urlPath string) (planeType, planeID, crNamespace, crName string, ok bool) {
	trimmed := strings.TrimPrefix(urlPath, GatewayPathPrefix)
	if trimmed == urlPath {
		return "", "", "", "", false
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) != 5 || parts[4] != gatewayMatchesSegment {
		return "", "", "", "", false
	}
	for _, part := range parts[:4] {
		if part == "" {
			return "", "", "", "", false
		}
	}

	return parts[0], parts[1], parts[2], parts[3], true
}
