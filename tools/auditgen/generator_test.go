// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/format"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

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
			kind, param, lastRaw := pathTail(tt.path)
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
		got, err := singularize(tt.kind)
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
	if _, err := singularize("autobuild"); err == nil {
		t.Error("expected an error for a kind with no trailing \"s\" and no override, got nil")
	}
}

func TestDeriveDefinition_Category(t *testing.T) {
	tests := []struct {
		method, path, id string
		wantCategory     string
	}{
		{"POST", "/api/v1/namespaces/{namespaceName}/authzroles", "CreateNamespaceRole", "CategoryAuthorization"},
		{"POST", "/api/v1/clusterauthzrolebindings", "CreateClusterRoleBinding", "CategoryAuthorization"},
		{"POST", "/api/v1/namespaces/{namespaceName}/projects", "CreateProject", "CategoryManagement"},
	}
	for _, tt := range tests {
		def, err := deriveDefinition(tt.method, tt.path, tt.id)
		if err != nil {
			t.Fatalf("deriveDefinition(%q, %q, %q) unexpected error: %v", tt.method, tt.path, tt.id, err)
		}
		if def.Category != tt.wantCategory {
			t.Errorf("deriveDefinition(%q).Category = %q, want %q", tt.id, def.Category, tt.wantCategory)
		}
	}
}

// TestCheckNoOrphanCategories_DetectsOrphan is a direct, isolated unit test
// for the mirror-image check to TestDeriveDefinition_UnknownKindErrors:
// resourceCategories is exhaustive in both directions now, so an entry naming
// a kind no operation actually uses (a resource kind renamed or removed from
// the API without cleaning up its now-stale entry here) must fail generation
// too, not just a kind with no entry.
func TestCheckNoOrphanCategories_DetectsOrphan(t *testing.T) {
	// A usedKinds set covering every real resourceCategories key: no orphan.
	allKinds := make(map[string]bool, len(resourceCategories))
	for kind := range resourceCategories {
		allKinds[kind] = true
	}
	if err := checkNoOrphanCategories(allKinds); err != nil {
		t.Fatalf("checkNoOrphanCategories(allKinds) unexpected error against a usedKinds set "+
			"covering every real resourceCategories key: %v", err)
	}

	// The same set minus one key: that one key is now an orphan.
	missingOne := make(map[string]bool, len(allKinds))
	for kind := range allKinds {
		missingOne[kind] = true
	}
	delete(missingOne, "projects")
	err := checkNoOrphanCategories(missingOne)
	if err == nil {
		t.Fatal("checkNoOrphanCategories(missingOne) = nil error, want an error naming the orphaned \"projects\" key")
	}
	if !strings.Contains(err.Error(), "projects") {
		t.Errorf("checkNoOrphanCategories(missingOne) error = %q, want it to name \"projects\"", err.Error())
	}
}

// TestBuildDefinitions_OrphanCategoryEntryErrors reproduces the exact repro
// from review: adding an entry for a resource kind that doesn't exist in the
// live spec (e.g. "nonexistentkinds") must fail buildDefinitions against the
// real embedded spec, not generate silently. Mutates the package-level
// resourceCategories for the duration of the test and restores it after.
func TestBuildDefinitions_OrphanCategoryEntryErrors(t *testing.T) {
	resourceCategories["nonexistentkinds"] = "CategoryManagement"
	t.Cleanup(func() { delete(resourceCategories, "nonexistentkinds") })

	swagger, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load swagger: %v", err)
	}

	_, err = buildDefinitions(swagger)
	if err == nil {
		t.Fatal("buildDefinitions() = nil error, want an error for an orphan resourceCategories entry")
	}
	if !strings.Contains(err.Error(), "nonexistentkinds") {
		t.Errorf("buildDefinitions() error = %q, want it to name the orphan entry %q", err.Error(), "nonexistentkinds")
	}
}

// TestDeriveDefinition_UnknownKindErrors guards the strict exit criterion the
// category gate promises: a resource kind with no entry in resourceCategories
// must fail generation rather than silently landing in CategoryManagement, so
// a new or renamed resource kind forces a deliberate category choice instead
// of an unnoticed default.
func TestDeriveDefinition_UnknownKindErrors(t *testing.T) {
	_, err := deriveDefinition("POST", "/api/v1/namespaces/{namespaceName}/newkinds", "CreateNewKind")
	if err == nil {
		t.Fatal("deriveDefinition() = nil error, want an error for a resource kind with no resourceCategories entry")
	}
	if !strings.Contains(err.Error(), "newkinds") || !strings.Contains(err.Error(), "resourceCategories") {
		t.Errorf("deriveDefinition() error = %q, want it to name the kind %q and resourceCategories", err.Error(), "newkinds")
	}
}

// TestDeriveDefinition_PropagatesSingularizeError is the case
// TestBuildDefinitions_AgainstLiveSpec can't reach: HandleAutoBuild's own path
// segment doesn't singularize mechanically. It's excluded from generation
// (excludedOperationIDs), so this hits deriveDefinition directly rather than
// through buildDefinitions.
func TestDeriveDefinition_PropagatesSingularizeError(t *testing.T) {
	_, err := deriveDefinition("POST", "/api/v1alpha1/autobuild", "HandleAutoBuild")
	if err == nil {
		t.Fatal("deriveDefinition() = nil error, want singularize's error for a kind with no trailing \"s\"")
	}
	if !strings.Contains(err.Error(), "autobuild") {
		t.Errorf("deriveDefinition() error = %q, want it to name the kind %q", err.Error(), "autobuild")
	}
}

// TestBuildDefinitions_MissingOperationIDErrors guards a route registered
// with no operationId at all — a spec-authoring mistake buildDefinitions must
// reject rather than generate an OperationDef with an empty ID that could
// never resolve at request time.
func TestBuildDefinitions_MissingOperationIDErrors(t *testing.T) {
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/widgets", "POST", &openapi3.Operation{})

	_, err := buildDefinitions(swagger)
	if err == nil {
		t.Fatal("buildDefinitions() = nil error, want an error for an operation with no operationId")
	}
	if !strings.Contains(err.Error(), "no operationId") {
		t.Errorf("buildDefinitions() error = %q, want it to mention a missing operationId", err.Error())
	}
}

// TestBuildDefinitions_WrapsDeriveDefinitionError guards that a
// deriveDefinition failure (here: an unknown resource kind) surfaces through
// buildDefinitions naming the operation and route, not just the bare
// underlying error.
func TestBuildDefinitions_WrapsDeriveDefinitionError(t *testing.T) {
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/newkinds", "POST",
		&openapi3.Operation{OperationID: "CreateNewKind"})

	_, err := buildDefinitions(swagger)
	if err == nil {
		t.Fatal("buildDefinitions() = nil error, want deriveDefinition's error propagated")
	}
	if !strings.Contains(err.Error(), "CreateNewKind") || !strings.Contains(err.Error(), "resourceCategories") {
		t.Errorf("buildDefinitions() error = %q, want it to name the operation and resourceCategories", err.Error())
	}
}

// TestBuildDefinitions_DuplicateOperationIDErrors guards against a
// copy-pasted operationId reaching two different routes — buildDefinitions
// must reject it rather than silently letting the second one's route become
// unreachable in the generated pattern map.
func TestBuildDefinitions_DuplicateOperationIDErrors(t *testing.T) {
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/projects", "POST",
		&openapi3.Operation{OperationID: "CreateProject"})
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/projects/{projectName}", "PUT",
		&openapi3.Operation{OperationID: "CreateProject"})

	_, err := buildDefinitions(swagger)
	if err == nil {
		t.Fatal("buildDefinitions() = nil error, want an error for a duplicate operationId")
	}
	if !strings.Contains(err.Error(), "CreateProject") {
		t.Errorf("buildDefinitions() error = %q, want it to name the duplicate operationId", err.Error())
	}
}

// TestRenderDefinitions_ProducesValidGo is the template's only test: a typo
// in fileTemplate would otherwise only surface as a make code.gen failure,
// not a test failure.
func TestRenderDefinitions_ProducesValidGo(t *testing.T) {
	defs := []operationDef{
		{ID: "CreateProject", Action: "create_project", ResourceType: "project", Category: "CategoryManagement"},
		{
			ID: "DeleteProject", Action: "delete_project", ResourceType: "project",
			Category: "CategoryManagement", RESTResourceParam: "projectName",
		},
	}

	src, err := renderDefinitions(defs)
	if err != nil {
		t.Fatalf("renderDefinitions() unexpected error: %v", err)
	}

	got := string(src)
	for _, want := range []string{
		"package audit", "CreateProject", "create_project", "audit.CategoryManagement", "projectName", "DO NOT EDIT",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderDefinitions() output missing %q:\n%s", want, got)
		}
	}
	if _, err := format.Source(src); err != nil {
		t.Errorf("renderDefinitions() output is not valid, gofmt'd Go: %v", err)
	}
}

func TestDeriveDefinition_TriggerVerb(t *testing.T) {
	def, err := deriveDefinition(
		"POST", "/api/v1alpha1/namespaces/{namespaceName}/releasebindings/{releaseBindingName}/trigger",
		"TriggerReleaseBindingCronJob",
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

// TestBuildDefinitions_AgainstLiveSpec pins the totals derived independently
// against the real embedded spec, so a spec change that shifts them fails
// here rather than silently reshaping definitions.gen.go.
//
// GenerateRelease counts as generated, not excluded: it persists a new
// ComponentRelease via k8sClient.Create
// (internal/openchoreo-api/services/component/service.go), so it gets a real
// definition (generateReleaseOverride) instead of an exemption.
func TestBuildDefinitions_AgainstLiveSpec(t *testing.T) {
	swagger, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load swagger: %v", err)
	}
	defs, err := buildDefinitions(swagger)
	if err != nil {
		t.Fatalf("buildDefinitions failed: %v", err)
	}

	const wantTotal = 112
	if len(defs) != wantTotal {
		t.Errorf("len(defs) = %d, want %d", len(defs), wantTotal)
	}

	var mgmt, authz int
	verbCount := map[string]int{}
	for _, d := range defs {
		switch d.Category {
		case "CategoryManagement":
			mgmt++
		case "CategoryAuthorization":
			authz++
		default:
			t.Errorf("operation %q has unrecognized category %q", d.ID, d.Category)
		}
		for _, verb := range []string{"create", "update", "delete", "trigger", "generate"} {
			if len(d.Action) > len(verb) && d.Action[:len(verb)+1] == verb+"_" {
				verbCount[verb]++
				break
			}
		}
	}

	if mgmt != 100 || authz != 12 {
		t.Errorf("category split = %d management / %d authorization, want 100/12", mgmt, authz)
	}
	if verbCount["create"] != 38 || verbCount["update"] != 34 || verbCount["delete"] != 38 ||
		verbCount["trigger"] != 1 || verbCount["generate"] != 1 {
		t.Errorf("verb split = %+v, want create:38 update:34 delete:38 trigger:1 generate:1", verbCount)
	}

	for _, excluded := range []string{"Evaluates", "HandleAutoBuild"} {
		for _, d := range defs {
			if d.ID == excluded {
				t.Errorf("excluded operation %q must not appear in generated defs", excluded)
			}
		}
	}

	found := false
	for _, d := range defs {
		if d.ID == "GenerateRelease" {
			found = true
			if d.ResourceType != "componentrelease" || d.RESTResourceParam != "" {
				t.Errorf("GenerateRelease = %+v, want ResourceType componentrelease, empty RESTResourceParam", d)
			}
		}
	}
	if !found {
		t.Error("GenerateRelease must appear in generated defs — it persists a ComponentRelease, see generateReleaseOverride")
	}
}
