// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

// Envelope is everything a surface adapter must supply to Emitter.Emit. It
// carries no HTTP or MCP types — an adapter translates its surface's
// request/response into an Envelope before calling Emit.
type Envelope struct {
	Origin    Origin
	Actor     Actor
	Result    Result
	Resource  *Resource
	Hierarchy Hierarchy
	RequestID string
	SourceIP  string
	Metadata  map[string]any
}
