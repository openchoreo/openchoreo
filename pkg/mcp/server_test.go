// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// captureHandler is a tiny http.Handler that records the request context it
// observed, so tests can assert that withSessionQueryParams populated it
// correctly.
type captureHandler struct {
	requestedToolsets map[tools.ToolsetType]bool
	hasRequested      bool
	filterByAuthz     bool
	hasFilter         bool
}

func (h *captureHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	h.requestedToolsets, h.hasRequested = tools.RequestedToolsetsFromContext(r.Context())
	h.filterByAuthz, h.hasFilter = tools.FilterByAuthzFromContext(r.Context())
}

func TestWithSessionQueryParamsParsesToolsets(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want map[tools.ToolsetType]bool
	}{
		{
			name: "single toolset",
			url:  "/mcp?toolsets=namespace",
			want: map[tools.ToolsetType]bool{tools.ToolsetNamespace: true},
		},
		{
			name: "multiple toolsets from issue example",
			url:  "/mcp?toolsets=namespace,component,pe",
			want: map[tools.ToolsetType]bool{
				tools.ToolsetNamespace: true,
				tools.ToolsetComponent: true,
				tools.ToolsetPE:        true,
			},
		},
		{
			name: "whitespace and empty entries ignored",
			url:  "/mcp?toolsets=namespace,%20,component,",
			want: map[tools.ToolsetType]bool{
				tools.ToolsetNamespace: true,
				tools.ToolsetComponent: true,
			},
		},
		{
			name: "unknown toolset preserved (silently matches no tools downstream)",
			url:  "/mcp?toolsets=foo",
			want: map[tools.ToolsetType]bool{tools.ToolsetType("foo"): true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &captureHandler{}
			h := withSessionQueryParams(cap)

			req := httptest.NewRequest(http.MethodPost, tt.url, http.NoBody)
			h.ServeHTTP(httptest.NewRecorder(), req)

			if !cap.hasRequested {
				t.Fatalf("expected requested toolsets to be set on context")
			}
			if !reflect.DeepEqual(cap.requestedToolsets, tt.want) {
				t.Errorf("requested toolsets = %v, want %v", cap.requestedToolsets, tt.want)
			}
		})
	}
}

func TestWithSessionQueryParamsAbsentToolsets(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "no query string", url: "/mcp"},
		{name: "param present but empty", url: "/mcp?toolsets="},
		{name: "param all whitespace and commas", url: "/mcp?toolsets=,,,%20,"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &captureHandler{}
			h := withSessionQueryParams(cap)
			req := httptest.NewRequest(http.MethodPost, tt.url, http.NoBody)
			h.ServeHTTP(httptest.NewRecorder(), req)
			if cap.hasRequested {
				t.Errorf("expected no narrowing, got %v", cap.requestedToolsets)
			}
		})
	}
}

func TestWithSessionQueryParamsParsesFilterByAuthz(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantSet   bool
		wantValue bool
	}{
		{name: "false", url: "/mcp?filterByAuthz=false", wantSet: true, wantValue: false},
		{name: "true", url: "/mcp?filterByAuthz=true", wantSet: true, wantValue: true},
		{name: "0", url: "/mcp?filterByAuthz=0", wantSet: true, wantValue: false},
		{name: "1", url: "/mcp?filterByAuthz=1", wantSet: true, wantValue: true},
		{name: "absent", url: "/mcp", wantSet: false},
		{name: "empty", url: "/mcp?filterByAuthz=", wantSet: false},
		{name: "invalid value treated as absent", url: "/mcp?filterByAuthz=maybe", wantSet: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &captureHandler{}
			h := withSessionQueryParams(cap)
			req := httptest.NewRequest(http.MethodPost, tt.url, http.NoBody)
			h.ServeHTTP(httptest.NewRecorder(), req)
			if cap.hasFilter != tt.wantSet {
				t.Fatalf("hasFilter = %v, want %v", cap.hasFilter, tt.wantSet)
			}
			if tt.wantSet && cap.filterByAuthz != tt.wantValue {
				t.Errorf("filterByAuthz = %v, want %v", cap.filterByAuthz, tt.wantValue)
			}
		})
	}
}

func TestWithSessionQueryParamsCombined(t *testing.T) {
	cap := &captureHandler{}
	h := withSessionQueryParams(cap)
	req := httptest.NewRequest(
		http.MethodPost,
		"/mcp?toolsets=namespace,component,pe&filterByAuthz=false",
		http.NoBody,
	)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !cap.hasRequested {
		t.Error("expected requested toolsets to be set")
	}
	if !cap.hasFilter || cap.filterByAuthz {
		t.Errorf("expected filterByAuthz=false, got hasFilter=%v value=%v", cap.hasFilter, cap.filterByAuthz)
	}
	want := map[tools.ToolsetType]bool{
		tools.ToolsetNamespace: true,
		tools.ToolsetComponent: true,
		tools.ToolsetPE:        true,
	}
	if !reflect.DeepEqual(cap.requestedToolsets, want) {
		t.Errorf("requested toolsets = %v, want %v", cap.requestedToolsets, want)
	}
}
