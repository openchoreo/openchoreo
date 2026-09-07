// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auditgen

import (
	"go/format"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// testActionSuffixSegments mirrors the real services' actionSuffixSegments
// for path-parsing tests that need a trigger-style suffix.
var testActionSuffixSegments = map[string]bool{
	"trigger":          true,
	"generate-release": true,
}

func TestPathTail(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		wantKind          string
		wantResourceParam string
		wantLastRaw       string
	}{
		{
			name:     "create route, no path param",
			path:     "/api/v1/namespaces/{namespaceName}/projects",
			wantKind: "projects", wantResourceParam: "", wantLastRaw: "projects",
		},
		{
			name:     "delete route with own path param",
			path:     "/api/v1/namespaces/{namespaceName}/dataplanes/{dpName}",
			wantKind: "dataplanes", wantResourceParam: "dpName", wantLastRaw: "{dpName}",
		},
		{
			name:     "trigger action suffix",
			path:     "/api/v1alpha1/namespaces/{namespaceName}/releasebindings/{releaseBindingName}/trigger",
			wantKind: "releasebindings", wantResourceParam: "releaseBindingName", wantLastRaw: "trigger",
		},
		{
			name:     "generate-release action suffix, no resource id (component is the parent)",
			path:     "/api/v1/namespaces/{namespaceName}/components/{componentName}/generate-release",
			wantKind: "components", wantResourceParam: "componentName", wantLastRaw: "generate-release",
		},
		{
			name:     "cluster-scoped resource, single path segment",
			path:     "/api/v1/clusterauthzroles/{name}",
			wantKind: "clusterauthzroles", wantResourceParam: "name", wantLastRaw: "{name}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, param, lastRaw := pathTail(tt.path, testActionSuffixSegments)
			if kind != tt.wantKind {
				t.Errorf("kindSegment = %q, want %q", kind, tt.wantKind)
			}
			if param != tt.wantResourceParam {
				t.Errorf("restResourceParam = %q, want %q", param, tt.wantResourceParam)
			}
			if lastRaw != tt.wantLastRaw {
				t.Errorf("lastRawSegment = %q, want %q", lastRaw, tt.wantLastRaw)
			}
		})
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"projects", "project"},
		{"dataplanes", "dataplane"},
		{"authzrolebindings", "authzrolebinding"},
		{"deploymentpipelines", "deploymentpipeline"},
		{"observabilityalertsnotificationchannels", "observabilityalertsnotificationchannel"},
	}
	for _, tt := range tests {
		got, err := singularize(tt.kind, nil)
		if err != nil {
			t.Errorf("singularize(%q) unexpected error: %v", tt.kind, err)
			continue
		}
		if got != tt.want {
			t.Errorf("singularize(%q) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestSingularize_NoTrailingSErrors(t *testing.T) {
	if _, err := singularize("autobuild", nil); err == nil {
		t.Error("expected an error for a kind with no trailing \"s\" and no override, got nil")
	}
}

// TestCheckNoOrphanCategories_DetectsOrphan is a direct, isolated unit test
// for the mirror-image check to a kind with no resourceCategories entry:
// resourceCategories is exhaustive in both directions, so an entry naming a
// kind no operation actually uses (a resource kind renamed or removed from
// the API without cleaning up its now-stale entry) must fail generation too,
// not just a kind with no entry.
func TestCheckNoOrphanCategories_DetectsOrphan(t *testing.T) {
	resourceCategories := map[string]string{
		"projects":   "CategoryManagement",
		"dataplanes": "CategoryManagement",
	}

	allKinds := map[string]bool{"projects": true, "dataplanes": true}
	if err := checkNoOrphanCategories(allKinds, resourceCategories); err != nil {
		t.Fatalf("checkNoOrphanCategories(allKinds) unexpected error against a usedKinds set "+
			"covering every real resourceCategories key: %v", err)
	}

	missingOne := map[string]bool{"dataplanes": true}
	err := checkNoOrphanCategories(missingOne, resourceCategories)
	if err == nil {
		t.Fatal("checkNoOrphanCategories(missingOne) = nil error, want an error naming the orphaned \"projects\" key")
	}
	if !strings.Contains(err.Error(), "projects") {
		t.Errorf("checkNoOrphanCategories(missingOne) error = %q, want it to name \"projects\"", err.Error())
	}
}

// TestDeriveDefinition_UnknownKindErrors guards the strict exit criterion the
// category gate promises: a resource kind with no entry in resourceCategories
// must fail generation rather than silently landing in CategoryManagement.
func TestDeriveDefinition_UnknownKindErrors(t *testing.T) {
	cfg := Config{ResourceCategories: map[string]string{}, ActionSuffixSegments: testActionSuffixSegments}
	_, err := deriveDefinition("POST", "/api/v1/namespaces/{namespaceName}/newkinds", "CreateNewKind", cfg)
	if err == nil {
		t.Fatal("deriveDefinition() = nil error, want an error for a resource kind with no resourceCategories entry")
	}
	if !strings.Contains(err.Error(), "newkinds") || !strings.Contains(err.Error(), "resourceCategories") {
		t.Errorf("deriveDefinition() error = %q, want it to name the kind %q and resourceCategories", err.Error(), "newkinds")
	}
}

// TestDeriveDefinition_PropagatesSingularizeError is a case buildDefinitions
// against a real spec can't reach: a kind whose own path segment doesn't
// singularize mechanically and has no override.
func TestDeriveDefinition_PropagatesSingularizeError(t *testing.T) {
	cfg := Config{ResourceCategories: map[string]string{"autobuild": "CategoryManagement"}}
	_, err := deriveDefinition("POST", "/api/v1alpha1/autobuild", "HandleAutoBuild", cfg)
	if err == nil {
		t.Fatal("deriveDefinition() = nil error, want singularize's error for a kind with no trailing \"s\"")
	}
	if !strings.Contains(err.Error(), "autobuild") {
		t.Errorf("deriveDefinition() error = %q, want it to name the kind %q", err.Error(), "autobuild")
	}
}

// TestBuildDefinitions_MissingOperationIDErrors guards a route registered
// with no operationId at all — a spec-authoring mistake BuildDefinitions must
// reject rather than generate an OperationDef with an empty ID that could
// never resolve at request time.
func TestBuildDefinitions_MissingOperationIDErrors(t *testing.T) {
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/widgets", "POST", &openapi3.Operation{})

	_, err := BuildDefinitions(swagger, Config{})
	if err == nil {
		t.Fatal("BuildDefinitions() = nil error, want an error for an operation with no operationId")
	}
	if !strings.Contains(err.Error(), "no operationId") {
		t.Errorf("BuildDefinitions() error = %q, want it to mention a missing operationId", err.Error())
	}
}

// TestBuildDefinitions_WrapsDeriveDefinitionError guards that a
// deriveDefinition failure (here: an unknown resource kind) surfaces through
// BuildDefinitions naming the operation and route, not just the bare
// underlying error.
func TestBuildDefinitions_WrapsDeriveDefinitionError(t *testing.T) {
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/newkinds", "POST",
		&openapi3.Operation{OperationID: "CreateNewKind"})

	_, err := BuildDefinitions(swagger, Config{ResourceCategories: map[string]string{}})
	if err == nil {
		t.Fatal("BuildDefinitions() = nil error, want deriveDefinition's error propagated")
	}
	if !strings.Contains(err.Error(), "CreateNewKind") || !strings.Contains(err.Error(), "resourceCategories") {
		t.Errorf("BuildDefinitions() error = %q, want it to name the operation and resourceCategories", err.Error())
	}
}

// TestBuildDefinitions_DuplicateOperationIDErrors guards against a
// copy-pasted operationId reaching two different routes — BuildDefinitions
// must reject it rather than silently letting the second one's route become
// unreachable in the generated pattern map.
func TestBuildDefinitions_DuplicateOperationIDErrors(t *testing.T) {
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/projects", "POST",
		&openapi3.Operation{OperationID: "CreateProject"})
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/projects/{projectName}", "PUT",
		&openapi3.Operation{OperationID: "CreateProject"})

	cfg := Config{ResourceCategories: map[string]string{"projects": "CategoryManagement"}}
	_, err := BuildDefinitions(swagger, cfg)
	if err == nil {
		t.Fatal("BuildDefinitions() = nil error, want an error for a duplicate operationId")
	}
	if !strings.Contains(err.Error(), "CreateProject") {
		t.Errorf("BuildDefinitions() error = %q, want it to name the duplicate operationId", err.Error())
	}
}

// TestRenderDefinitions_ProducesValidGo is the template's only test: a typo
// in fileTemplate would otherwise only surface as a make code.gen failure,
// not a test failure.
func TestRenderDefinitions_ProducesValidGo(t *testing.T) {
	defs := []OperationDef{
		{ID: "CreateProject", Action: "create_project", ResourceType: "project", Category: "CategoryManagement"},
		{
			ID: "DeleteProject", Action: "delete_project", ResourceType: "project",
			Category: "CategoryManagement", RESTResourceParam: "projectName",
		},
	}

	src, err := RenderDefinitions(defs, RenderOptions{})
	if err != nil {
		t.Fatalf("RenderDefinitions() unexpected error: %v", err)
	}

	got := string(src)
	for _, want := range []string{
		"package audit", "CreateProject", "create_project", "audit.CategoryManagement", "projectName", "DO NOT EDIT",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderDefinitions() output missing %q:\n%s", want, got)
		}
	}
	if _, err := format.Source(src); err != nil {
		t.Errorf("RenderDefinitions() output is not valid, gofmt'd Go: %v", err)
	}
}

func TestDeriveDefinition_TriggerVerb(t *testing.T) {
	cfg := Config{
		ResourceCategories:   map[string]string{"releasebindings": "CategoryManagement"},
		ActionSuffixSegments: testActionSuffixSegments,
	}
	def, err := deriveDefinition(
		"POST", "/api/v1alpha1/namespaces/{namespaceName}/releasebindings/{releaseBindingName}/trigger",
		"TriggerReleaseBindingCronJob", cfg,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Action comes from word-splitting the operationId (actionFromOperationID),
	// not from verb+"_"+ResourceType — so it includes "CronJob" even though
	// ResourceType (still path-segment-derived) does not. See
	// actionFromOperationID's doc comment.
	if def.Action != "trigger_release_binding_cron_job" {
		t.Errorf("Action = %q, want %q", def.Action, "trigger_release_binding_cron_job")
	}
	if def.ResourceType != "releasebinding" {
		t.Errorf("ResourceType = %q, want %q", def.ResourceType, "releasebinding")
	}
	if def.RESTResourceParam != "releaseBindingName" {
		t.Errorf("RESTResourceParam = %q, want %q", def.RESTResourceParam, "releaseBindingName")
	}
}
