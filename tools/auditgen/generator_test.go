// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/tools/internal/auditgen"
)

func TestDeriveDefinition_Category(t *testing.T) {
	tests := []struct {
		method, path, id string
		wantCategory     string
	}{
		{"POST", "/api/v1/namespaces/{namespaceName}/authzroles", "CreateNamespaceRole", "CategoryAuthorization"},
		{"POST", "/api/v1/clusterauthzrolebindings", "CreateClusterRoleBinding", "CategoryAuthorization"},
		{"POST", "/api/v1/namespaces/{namespaceName}/projects", "CreateProject", "CategoryManagement"},
	}
	swagger, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load swagger: %v", err)
	}
	defs, err := auditgen.BuildDefinitions(swagger, apiConfig())
	if err != nil {
		t.Fatalf("BuildDefinitions failed: %v", err)
	}
	byID := make(map[string]auditgen.OperationDef, len(defs))
	for _, d := range defs {
		byID[d.ID] = d
	}
	for _, tt := range tests {
		def, ok := byID[tt.id]
		if !ok {
			t.Fatalf("operation %q not found in generated defs", tt.id)
		}
		if def.Category != tt.wantCategory {
			t.Errorf("defs[%q].Category = %q, want %q", tt.id, def.Category, tt.wantCategory)
		}
	}
}

// TestDeriveDefinition_UnknownKindErrors guards the strict exit criterion the
// category gate promises against openchoreo-api's real config: a resource
// kind with no entry in resourceCategories must fail generation rather than
// silently landing in CategoryManagement.
func TestDeriveDefinition_UnknownKindErrors(t *testing.T) {
	if _, ok := apiResourceCategories["newkinds"]; ok {
		t.Fatal("test setup invalid: \"newkinds\" must not be a real resourceCategories entry")
	}

	syntheticSwagger := buildSyntheticSwaggerWithNewKind(t)
	_, err := auditgen.BuildDefinitions(syntheticSwagger, apiConfig())
	if err == nil {
		t.Fatal("BuildDefinitions() = nil error, want an error for a resource kind with no resourceCategories entry")
	}
	if !strings.Contains(err.Error(), "newkinds") || !strings.Contains(err.Error(), "resourceCategories") {
		t.Errorf("BuildDefinitions() error = %q, want it to name the kind %q and resourceCategories", err.Error(), "newkinds")
	}
}

func buildSyntheticSwaggerWithNewKind(t *testing.T) *openapi3.T {
	t.Helper()
	swagger := &openapi3.T{Paths: openapi3.NewPaths()}
	swagger.AddOperation("/api/v1/namespaces/{namespaceName}/newkinds", "POST",
		&openapi3.Operation{OperationID: "CreateNewKind"})
	return swagger
}

// TestBuildDefinitions_OrphanCategoryEntryErrors reproduces the exact repro
// from review: adding an entry for a resource kind that doesn't exist in the
// live spec (e.g. "nonexistentkinds") must fail BuildDefinitions against the
// real embedded spec, not generate silently. Mutates the package-level
// resourceCategories for the duration of the test and restores it after.
func TestBuildDefinitions_OrphanCategoryEntryErrors(t *testing.T) {
	apiResourceCategories["nonexistentkinds"] = "CategoryManagement"
	t.Cleanup(func() { delete(apiResourceCategories, "nonexistentkinds") })

	swagger, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("failed to load swagger: %v", err)
	}

	_, err = auditgen.BuildDefinitions(swagger, apiConfig())
	if err == nil {
		t.Fatal("BuildDefinitions() = nil error, want an error for an orphan resourceCategories entry")
	}
	if !strings.Contains(err.Error(), "nonexistentkinds") {
		t.Errorf("BuildDefinitions() error = %q, want it to name the orphan entry %q", err.Error(), "nonexistentkinds")
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
	defs, err := auditgen.BuildDefinitions(swagger, apiConfig())
	if err != nil {
		t.Fatalf("BuildDefinitions failed: %v", err)
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
