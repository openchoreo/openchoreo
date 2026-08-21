// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMatchRequestJSONRoundTrip pins the wire field names and the omitempty
// behavior the agent (Task 5) and the tunnel client (Task 6) encode against.
func TestMatchRequestJSONRoundTrip(t *testing.T) {
	criteria, err := json.Marshal(LabelSelectorCriteria{
		MatchLabels: map[string]string{
			"app.kubernetes.io/instance": TokenParentName,
			"openchoreo.dev/namespace":   TokenParentNamespace,
		},
		Namespaces: []string{"default", "observability"},
	})
	if err != nil {
		t.Fatalf("marshal criteria: %v", err)
	}

	req := MatchRequest{
		Version: Version,
		Queries: []MatchQuery{
			{
				ID:       "q1",
				Matcher:  MatcherLabelSelector,
				Criteria: criteria,
				Parents: []ParentRef{
					{UID: "uid-1", Namespace: "default", Name: "my-app"},
				},
				Child:        ChildKind{Version: "v1", Kind: "Pod", Resource: "pods"},
				MetadataOnly: true,
			},
			{
				ID:      "q2",
				Matcher: MatcherOwnerRef,
				Parents: []ParentRef{
					{UID: "uid-2", Namespace: "default", Name: "my-app-rs"},
				},
				Child: ChildKind{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	encoded := string(data)

	t.Run("encoding names and omits fields", func(t *testing.T) {
		assertMatchRequestEncoding(t, encoded)
	})
	t.Run("decoding preserves values", func(t *testing.T) {
		assertMatchRequestDecoding(t, data)
	})
}

// assertMatchRequestEncoding pins the wire field names present and the
// omitempty fields absent in a marshaled MatchRequest.
func assertMatchRequestEncoding(t *testing.T, encoded string) {
	t.Helper()

	for _, field := range []string{
		`"version"`, `"queries"`, `"id"`, `"matcher"`, `"criteria"`, `"parents"`,
		`"uid"`, `"namespace"`, `"name"`, `"child"`, `"group"`, `"kind"`,
		`"resource"`, `"metadataOnly"`, `"matchLabels"`, `"namespaces"`,
	} {
		if !strings.Contains(encoded, field) {
			t.Errorf("marshaled request is missing field %s: %s", field, encoded)
		}
	}

	// The ownerRef query carries no criteria; an empty Criteria must be dropped
	// rather than encoded as null, which would not decode as "no criteria".
	if strings.Count(encoded, `"criteria"`) != 1 {
		t.Errorf("expected exactly one criteria field (labelSelector only), got: %s", encoded)
	}
	// Group is empty for the core-group Pod child and must be omitted.
	if strings.Count(encoded, `"group"`) != 1 {
		t.Errorf("expected group to be omitted for the core API group, got: %s", encoded)
	}
	// MetadataOnly is false on the ownerRef query and must be omitted.
	if strings.Count(encoded, `"metadataOnly"`) != 1 {
		t.Errorf("expected metadataOnly to be omitted when false, got: %s", encoded)
	}
	// The control plane never sets Limit, so it must not reach the wire at all:
	// the agent reads its absence as DefaultMatchLimit.
	if strings.Contains(encoded, `"limit"`) {
		t.Errorf("expected limit to be omitted when unset, got: %s", encoded)
	}
}

// assertMatchRequestDecoding pins that a marshaled MatchRequest decodes back to
// the values and unsubstituted tokens the agent and tunnel client expect.
func assertMatchRequestDecoding(t *testing.T, data []byte) {
	t.Helper()

	var decoded MatchRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if decoded.Version != Version {
		t.Errorf("version = %q, want %q", decoded.Version, Version)
	}
	if len(decoded.Queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(decoded.Queries))
	}

	labelQuery := decoded.Queries[0]
	if labelQuery.ID != "q1" || labelQuery.Matcher != MatcherLabelSelector {
		t.Errorf("unexpected labelSelector query: %+v", labelQuery)
	}
	if !labelQuery.MetadataOnly {
		t.Error("metadataOnly did not survive the round trip")
	}
	if labelQuery.Limit != 0 {
		t.Errorf("limit = %d, want 0 (unset)", labelQuery.Limit)
	}
	if len(labelQuery.Parents) != 1 || labelQuery.Parents[0].UID != "uid-1" ||
		labelQuery.Parents[0].Namespace != "default" || labelQuery.Parents[0].Name != "my-app" {
		t.Errorf("unexpected parents: %+v", labelQuery.Parents)
	}
	if labelQuery.Child.Group != "" || labelQuery.Child.Kind != "Pod" || labelQuery.Child.Resource != "pods" {
		t.Errorf("unexpected child kind: %+v", labelQuery.Child)
	}

	ownerQuery := decoded.Queries[1]
	if len(ownerQuery.Criteria) != 0 {
		t.Errorf("ownerRef query criteria = %q, want empty", ownerQuery.Criteria)
	}
	if ownerQuery.MetadataOnly {
		t.Error("ownerRef query metadataOnly = true, want false")
	}

	// Templates cross the wire untouched: the agent substitutes per parent, so a
	// decoded criteria block must still hold the raw token strings.
	var gotCriteria LabelSelectorCriteria
	if err := json.Unmarshal(labelQuery.Criteria, &gotCriteria); err != nil {
		t.Fatalf("unmarshal criteria: %v", err)
	}
	if got := gotCriteria.MatchLabels["app.kubernetes.io/instance"]; got != TokenParentName {
		t.Errorf("instance label = %q, want the unsubstituted token %q", got, TokenParentName)
	}
	if got := gotCriteria.MatchLabels["openchoreo.dev/namespace"]; got != TokenParentNamespace {
		t.Errorf("namespace label = %q, want the unsubstituted token %q", got, TokenParentNamespace)
	}
	if len(gotCriteria.Namespaces) != 2 || gotCriteria.Namespaces[0] != "default" ||
		gotCriteria.Namespaces[1] != "observability" {
		t.Errorf("unexpected namespaces: %v", gotCriteria.Namespaces)
	}
}

// TestMatchQueryLimitWireName pins the wire name of the reserved Limit field.
// No control-plane caller sets it today, so the round-trip test above cannot
// cover it, but the agent still honors it and a rename would break that.
func TestMatchQueryLimitWireName(t *testing.T) {
	data, err := json.Marshal(MatchQuery{ID: "q1", Matcher: MatcherOwnerRef, Limit: 2})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	if !strings.Contains(string(data), `"limit":2`) {
		t.Errorf("marshaled query is missing limit: %s", data)
	}
}

// TestMatchResponseJSONRoundTrip pins the response field names, including
// parentUIDs, which the control-plane walk (Task 8) keys children by.
func TestMatchResponseJSONRoundTrip(t *testing.T) {
	resp := MatchResponse{
		Version: Version,
		Results: []MatchResult{
			{
				ID: "q1",
				Matches: []MatchedObject{
					{
						ParentUIDs: []string{"uid-1", "uid-2"},
						Object:     json.RawMessage(`{"kind":"Pod"}`),
					},
				},
				Truncated: true,
			},
			{
				ID:    "q2",
				Error: &MatchError{Code: CodeForbidden, Message: "pods is forbidden"},
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	encoded := string(data)

	for _, field := range []string{
		`"version"`, `"results"`, `"id"`, `"matches"`, `"parentUIDs"`,
		`"object"`, `"truncated"`, `"error"`, `"code"`, `"message"`,
	} {
		if !strings.Contains(encoded, field) {
			t.Errorf("marshaled response is missing field %s: %s", field, encoded)
		}
	}

	// The failed result has no matches and is not truncated; the successful one
	// has no error. Neither may be encoded as a null the reader must handle.
	if strings.Count(encoded, `"matches"`) != 1 {
		t.Errorf("expected empty matches to be omitted, got: %s", encoded)
	}
	if strings.Count(encoded, `"truncated"`) != 1 {
		t.Errorf("expected truncated to be omitted when false, got: %s", encoded)
	}
	if strings.Count(encoded, `"error"`) != 1 {
		t.Errorf("expected a nil error to be omitted, got: %s", encoded)
	}

	var decoded MatchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(decoded.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(decoded.Results))
	}

	ok := decoded.Results[0]
	if len(ok.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(ok.Matches))
	}
	if len(ok.Matches[0].ParentUIDs) != 2 || ok.Matches[0].ParentUIDs[0] != "uid-1" {
		t.Errorf("unexpected parent UIDs: %v", ok.Matches[0].ParentUIDs)
	}
	if string(ok.Matches[0].Object) != `{"kind":"Pod"}` {
		t.Errorf("object = %s, want the raw bytes preserved", ok.Matches[0].Object)
	}
	if !ok.Truncated {
		t.Error("truncated did not survive the round trip")
	}
	if ok.Error != nil {
		t.Errorf("unexpected error on successful result: %+v", ok.Error)
	}

	failed := decoded.Results[1]
	if failed.Error == nil || failed.Error.Code != CodeForbidden {
		t.Errorf("unexpected error on failed result: %+v", failed.Error)
	}
	if len(failed.Matches) != 0 {
		t.Errorf("expected no matches on the failed result, got %v", failed.Matches)
	}
}

// TestMaxSelectorNamespacesIsEight pins this package's own constant to its
// literal value. This is the cap the agent enforces, and config's
// maxSelectorNamespaces aliases it so config can never validate a rule the agent
// would reject as InvalidQuery; config keeps a mirror-image pin in case that
// alias is ever replaced by a literal.
func TestMaxSelectorNamespacesIsEight(t *testing.T) {
	if MaxSelectorNamespaces != 8 {
		t.Errorf("MaxSelectorNamespaces = %d, want 8; config's maxSelectorNamespaces must be changed to match",
			MaxSelectorNamespaces)
	}
}

func TestSubstituteParentTokens(t *testing.T) {
	const (
		parentName      = "my-app"
		parentNamespace = "default"
	)

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{
			name:  "name token alone",
			value: TokenParentName,
			want:  parentName,
		},
		{
			name:  "namespace token alone",
			value: TokenParentNamespace,
			want:  parentNamespace,
		},
		{
			name:  "both tokens in one value",
			value: TokenParentNamespace + "/" + TokenParentName,
			want:  "default/my-app",
		},
		{
			name:  "token embedded in literal text",
			value: "release-" + TokenParentName + "-primary",
			want:  "release-my-app-primary",
		},
		{
			name:  "same token repeated",
			value: TokenParentName + "-" + TokenParentName,
			want:  "my-app-my-app",
		},
		{
			name:  "no tokens passes through unchanged",
			value: "plain-label-value",
			want:  "plain-label-value",
		},
		{
			name:  "empty value",
			value: "",
			want:  "",
		},
		{
			name:  "lone dollar is not a token",
			value: "cost-$100",
			want:  "cost-$100",
		},
		{
			name:    "unknown token path",
			value:   "${parent.spec.foo}",
			wantErr: true,
		},
		{
			name:    "unknown token after a valid one",
			value:   TokenParentName + "-${parent.spec.foo}",
			wantErr: true,
		},
		{
			name:    "stray unterminated dollar brace",
			value:   "${parent.metadata.name",
			wantErr: true,
		},
		{
			name:    "unterminated token after literal text",
			value:   "prefix-${",
			wantErr: true,
		},
		{
			name:    "token with trailing whitespace inside braces",
			value:   "${parent.metadata.name }",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SubstituteParentTokens(tt.value, parentName, parentNamespace)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %q", tt.value, got)
				}
				// Fail closed: nothing partially substituted leaks out on error.
				if got != "" {
					t.Errorf("expected an empty result on error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("SubstituteParentTokens(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestSubstituteParentTokensErrorNamesToken checks that the error points the
// config author at the offending text rather than only at the whole value.
func TestSubstituteParentTokensErrorNamesToken(t *testing.T) {
	_, err := SubstituteParentTokens("app-${parent.spec.foo}-x", "my-app", "default")
	if err == nil {
		t.Fatal("expected an error for an unknown token")
	}
	if !strings.Contains(err.Error(), "${parent.spec.foo}") {
		t.Errorf("error %q does not name the offending token", err)
	}
}

func TestParentFieldUse(t *testing.T) {
	tests := []struct {
		name             string
		matchLabels      map[string]string
		wantName, wantNS bool
	}{
		{"no tokens", map[string]string{"app": "static"}, false, false},
		{"name token", map[string]string{"owner": TokenParentName}, true, false},
		{"namespace token", map[string]string{"ns": TokenParentNamespace}, false, true},
		{"both tokens one value", map[string]string{"id": TokenParentName + "." + TokenParentNamespace}, true, true},
		{"embedded in literal text", map[string]string{"owner": "gw-" + TokenParentName + "-proxy"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &LabelSelectorCriteria{MatchLabels: tt.matchLabels}
			gotName, gotNS := c.ParentFieldUse()
			if gotName != tt.wantName || gotNS != tt.wantNS {
				t.Errorf("ParentFieldUse() = (%v, %v), want (%v, %v)", gotName, gotNS, tt.wantName, tt.wantNS)
			}
		})
	}
}

// TestSubstituteParentTokensEmptyParentValues documents that substitution is
// purely textual: an empty parent field yields an empty substitution, it is not
// an error at this layer.
func TestSubstituteParentTokensEmptyParentValues(t *testing.T) {
	got, err := SubstituteParentTokens("ns-"+TokenParentNamespace, "my-app", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ns-" {
		t.Errorf("got %q, want %q", got, "ns-")
	}
}
