// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package protocol defines the wire contract between the control plane and the
// cluster agent for resource tree child discovery: the request and response
// types carried over the tunnel, the guardrail limits both sides enforce, and
// the single shared implementation of parent token substitution.
//
// It is deliberately dependency-free — no Kubernetes client types, no config
// package — so both sides can depend on it without dragging their internals
// into each other.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	Version              = "v1"
	Target               = "resource-tree" // HTTPTunnelRequest.Target
	PathMatches          = "/v1/matches"   // HTTPTunnelRequest.Path
	MatcherOwnerRef      = "ownerRef"
	MatcherLabelSelector = "labelSelector"

	// TokenParentName and TokenParentNamespace are the only substitution tokens
	// recognized in LabelSelectorCriteria.MatchLabels values; the agent
	// substitutes them per parent.
	TokenParentName      = "${parent.metadata.name}"
	TokenParentNamespace = "${parent.metadata.namespace}"

	// LastAppliedConfigAnnotation is kubectl's record of the object as last
	// applied. Its value is the WHOLE serialized resource, so on a Secret it
	// carries the data block verbatim — which survives both the agent's
	// metadata-only trim and the API's top-level data strip, since neither looks
	// inside annotations. Both sides drop it by name, and both name it from here
	// so the two strips cannot drift apart.
	LastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

	CodeForbidden          = "Forbidden"
	CodeUnsupportedMatcher = "UnsupportedMatcher"
	CodeUnsupportedVersion = "UnsupportedVersion"
	CodeInvalidQuery       = "InvalidQuery"
	CodeLimitExceeded      = "LimitExceeded"
	CodeInternal           = "Internal"

	DefaultMatchLimit    = 500
	ListPageSize         = 200
	MaxQueriesPerRequest = 64  // request exceeding this → HTTP 400
	MaxParentsPerQuery   = 256 // query exceeding this → InvalidQuery
	// MaxSelectorNamespaces caps LabelSelectorCriteria.Namespaces; a criteria
	// block exceeding it → InvalidQuery.
	MaxSelectorNamespaces = 8
)

// tokenOpen introduces a substitution token. Every occurrence must open one of
// the two recognized tokens; see SubstituteParentTokens.
const tokenOpen = "${"

type MatchRequest struct {
	Version string       `json:"version"`
	Queries []MatchQuery `json:"queries"`
}

// MatchQuery asks for the children of a set of parents for ONE child kind
// under ONE matcher. Criteria carries matcher-specific parameters as raw
// JSON so new matchers need no protocol change; it MUST be empty for
// ownerRef and MUST decode to LabelSelectorCriteria for labelSelector
// (validated agent-side). IDs must be unique within a request. There is no
// Namespaces field on the query: for ownerRef the agent derives the
// namespace set from Parents[].Namespace; cross-namespace matchers scope
// namespaces inside their own Criteria block (labelSelector does).
type MatchQuery struct {
	ID           string          `json:"id"`
	Matcher      string          `json:"matcher"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
	Parents      []ParentRef     `json:"parents"`
	Child        ChildKind       `json:"child"`
	MetadataOnly bool            `json:"metadataOnly,omitempty"`
	// Limit caps this query's matches. Zero means DefaultMatchLimit, and the
	// agent clamps any out-of-range value back to it, so omitting the field is
	// always safe. Reserved for callers that need a smaller batch; the control
	// plane currently always omits it.
	Limit int `json:"limit,omitempty"`
}

type ParentRef struct {
	UID       string `json:"uid"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type ChildKind struct {
	Group    string `json:"group,omitempty"`
	Version  string `json:"version"`
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}

// LabelSelectorCriteria is the Criteria payload for matcher labelSelector.
// MatchLabels values travel UNSUBSTITUTED; the agent replaces
// TokenParentName/TokenParentNamespace per parent, so one query still
// serves many parents. Namespaces empty means each parent's own namespace;
// entries are literal names (no wildcard), at most MaxSelectorNamespaces.
type LabelSelectorCriteria struct {
	MatchLabels map[string]string `json:"matchLabels"`
	Namespaces  []string          `json:"namespaces,omitempty"`
}

// ParentFieldUse reports which parent fields the criteria's matchLabels values
// derive from. Exact rather than heuristic: token validation has already
// rejected any other ${...} occurrence, and the two tokens diverge at the byte
// after "name", so a Contains check cannot false-positive.
func (c *LabelSelectorCriteria) ParentFieldUse() (usesName, usesNamespace bool) {
	for _, value := range c.MatchLabels {
		if strings.Contains(value, TokenParentName) {
			usesName = true
		}
		if strings.Contains(value, TokenParentNamespace) {
			usesNamespace = true
		}
	}
	return usesName, usesNamespace
}

// SubstituteParentTokens replaces TokenParentName/TokenParentNamespace in one
// matchLabels value. ONE implementation shared by the agent handler (Task 5)
// and the API's legacy fallback (Task 8) so token semantics cannot drift.
// Any other ${ occurrence returns an error — fail closed, never passed
// through literally.
//
// Substitution is textual, not an expression language: both tokens may appear
// in the same value, repeatedly, and embedded in surrounding literal text. A
// value with no tokens is returned unchanged. An unknown path such as
// ${parent.spec.foo}, and an unterminated ${, are both errors; on error the
// returned value is empty so a partial substitution can never be used.
func SubstituteParentTokens(value, parentName, parentNamespace string) (string, error) {
	if !strings.Contains(value, tokenOpen) {
		return value, nil
	}

	var out strings.Builder
	out.Grow(len(value))

	rest := value
	for {
		start := strings.Index(rest, tokenOpen)
		if start < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}

		out.WriteString(rest[:start])
		rest = rest[start:]

		switch {
		case strings.HasPrefix(rest, TokenParentName):
			out.WriteString(parentName)
			rest = rest[len(TokenParentName):]
		case strings.HasPrefix(rest, TokenParentNamespace):
			out.WriteString(parentNamespace)
			rest = rest[len(TokenParentNamespace):]
		default:
			return "", fmt.Errorf("unknown substitution token %q in value %q; only %s and %s are supported",
				tokenAt(rest), value, TokenParentName, TokenParentNamespace)
		}
	}
}

// tokenAt returns the ${...} occurrence at the start of rest so the error can
// name what the author wrote. An unterminated token yields the whole remainder,
// matching how the config package reports one.
func tokenAt(rest string) string {
	if end := strings.Index(rest, "}"); end >= 0 {
		return rest[:end+1]
	}
	return rest
}

type MatchResponse struct {
	Version string        `json:"version"`
	Results []MatchResult `json:"results"`
}

type MatchResult struct {
	ID        string          `json:"id"`
	Matches   []MatchedObject `json:"matches,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	Error     *MatchError     `json:"error,omitempty"`
}

type MatchedObject struct {
	ParentUIDs []string        `json:"parentUIDs"`
	Object     json.RawMessage `json:"object"`
}

type MatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// knownErrorCodes is the closed set of MatchError codes this protocol version
// defines. It is the authority for IsKnownErrorCode.
var knownErrorCodes = map[string]struct{}{
	CodeForbidden:          {},
	CodeUnsupportedMatcher: {},
	CodeUnsupportedVersion: {},
	CodeInvalidQuery:       {},
	CodeLimitExceeded:      {},
	CodeInternal:           {},
}

// IsKnownErrorCode reports whether code is a MatchError code this protocol
// version defines. A peer that returns any other code — or an empty one — is
// not speaking this protocol, so the client rejects the whole response rather
// than downgrading an unrecognized code to a generic error and losing an
// actionable classification such as Forbidden.
func IsKnownErrorCode(code string) bool {
	_, ok := knownErrorCodes[code]
	return ok
}
