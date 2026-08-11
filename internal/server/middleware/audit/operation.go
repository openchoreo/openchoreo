// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

// Operation is a surface-neutral auditable operation. The OpenAPI operationId
// is the canonical key: on the REST surface it is 1:1 with a route, and MCP
// tools bind onto it through a declared table rather than a second set of
// action definitions.
type Operation struct {
	// ID is the OpenAPI operationId, e.g. "createProject".
	ID string
	// Action is the semantic audit action, e.g. "create_project".
	Action string
	// ResourceType is derived from the operation so every event and every
	// PolicySet selector can key on it, independent of whether a handler ever
	// calls SetResource, e.g. "projects".
	ResourceType string
	// Category is stamped from the operation's resource kind. It is not
	// operator-configurable — see PolicySet's validation of the "category" key.
	Category Category
}
