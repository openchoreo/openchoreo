// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestToEndpointSpecVisibility(t *testing.T) {
	got, err := toEndpointSpecVisibility([]string{"project", "namespace", "internal", "external"})
	if err != nil {
		t.Fatalf("expected every Endpoint visibility to be accepted, got %v", err)
	}
	want := []gen.EndpointSpecVisibility{
		gen.EndpointSpecVisibilityProject, gen.EndpointSpecVisibilityNamespace,
		gen.EndpointSpecVisibilityInternal, gen.EndpointSpecVisibilityExternal,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	_, err = toEndpointSpecVisibility([]string{"external", "public"})
	const wantErr = `visibility "public" must be one of: project, namespace, internal, external`
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("expected an unknown visibility to be rejected, got %v", err)
	}
}

func TestEndpointVisibilityPropertyListsProject(t *testing.T) {
	items := endpointVisibilityProperty("scopes")["items"].(map[string]any)
	if got := items["enum"]; !reflect.DeepEqual(got, []string{"project", "namespace", "internal", "external"}) {
		t.Errorf("visibility enum = %v", got)
	}
}
