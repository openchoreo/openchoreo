// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const testSpecYAML = `
openapi: 3.0.0
info:
  title: test
  version: "1"
paths:
  /projects:
    post:
      operationId: CreateProject
      responses:
        "200":
          description: ok
  /projects/{name}:
    put:
      operationId: UpdateProject
      responses:
        "200":
          description: ok
    delete:
      operationId: DeleteProject
      responses:
        "200":
          description: ok
`

func loadTestSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(testSpecYAML))
	if err != nil {
		t.Fatalf("failed to load test spec: %v", err)
	}
	return doc
}

func TestBuildPatternMap_Success(t *testing.T) {
	swagger := loadTestSpec(t)
	ops := []Operation{
		{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement},
		{
			ID: "UpdateProject", Action: "update_project", ResourceType: "project", Category: CategoryManagement,
			RESTResourceParam: "name",
		},
	}

	patternMap, err := BuildPatternMap(ops, swagger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patternMap) != 2 {
		t.Fatalf("len(patternMap) = %d, want 2", len(patternMap))
	}

	op, ok := patternMap[testProjectPattern]
	if !ok {
		t.Fatal(`patternMap["POST /projects"] missing`)
	}
	if op.ID != testProjectOpID {
		t.Errorf(`patternMap["POST /projects"].ID = %q, want "CreateProject"`, op.ID)
	}

	op, ok = patternMap["PUT /projects/{name}"]
	if !ok {
		t.Fatal(`patternMap["PUT /projects/{name}"] missing`)
	}
	if op.ID != "UpdateProject" {
		t.Errorf(`patternMap["PUT /projects/{name}"].ID = %q, want "UpdateProject"`, op.ID)
	}
}

func TestBuildPatternMap_UnknownOperationID(t *testing.T) {
	swagger := loadTestSpec(t)
	ops := []Operation{
		{ID: "CreateWidget", Action: "create_widget", ResourceType: "widgets", Category: CategoryManagement},
	}

	_, err := BuildPatternMap(ops, swagger)
	if err == nil {
		t.Fatal("expected an error for an operationId with no matching route, got nil")
	}
	if !strings.Contains(err.Error(), "CreateWidget") {
		t.Errorf("error = %q, want it to name the missing operationId CreateWidget", err.Error())
	}
}

// TestBuildPatternMap_RESTResourceParamMismatch guards against a typo'd
// RESTResourceParam surfacing as a silently empty resource.name on denied or
// failed events, instead of a construction-time error — the same class of
// protection BuildPatternMap already gives operationId and pattern
// collisions.
func TestBuildPatternMap_RESTResourceParamMismatch(t *testing.T) {
	swagger := loadTestSpec(t)
	ops := []Operation{
		{
			ID: "UpdateProject", Action: "update_project", ResourceType: "project", Category: CategoryManagement,
			RESTResourceParam: "projectName", // route param is actually "name"
		},
	}

	_, err := BuildPatternMap(ops, swagger)
	if err == nil {
		t.Fatal("expected an error for a RESTResourceParam with no matching path parameter, got nil")
	}
	if !strings.Contains(err.Error(), "projectName") {
		t.Errorf("error = %q, want it to name the mismatched RESTResourceParam projectName", err.Error())
	}
}

// TestBuildPatternMap_PatternCollision covers the realistic way this fires:
// a caller's own ops table lists the same operationId twice (e.g. a
// copy-pasted OperationDef entry with a forgotten ID change), so both
// resolve to the same spec route.
func TestBuildPatternMap_PatternCollision(t *testing.T) {
	swagger := loadTestSpec(t)
	ops := []Operation{
		{ID: "DeleteProject", Action: "delete_project", ResourceType: "project", Category: CategoryManagement},
		{ID: "DeleteProject", Action: "delete_project_dup", ResourceType: "project", Category: CategoryManagement},
	}

	_, err := BuildPatternMap(ops, swagger)
	if err == nil {
		t.Fatal("expected a pattern collision error, got nil")
	}
	if !strings.Contains(err.Error(), "DELETE /projects/{name}") {
		t.Errorf("error = %q, want it to name the colliding pattern", err.Error())
	}
}

// TestBuildPatternMap_SkipsNotInOpenAPISpec guards exec/wirelogs' construction
// path: an operation with no route in the spec at all must be skipped rather
// than failing construction (the outcome TestBuildPatternMap_UnknownOperationID
// covers for every other operation), and must not appear in the result — its
// caller (NewExecWirelogsAuditMiddleware) owns its own pattern-map entry.
func TestBuildPatternMap_SkipsNotInOpenAPISpec(t *testing.T) {
	swagger := loadTestSpec(t)
	ops := []Operation{
		{ID: testProjectOpID, Action: "create_project", ResourceType: "project", Category: CategoryManagement},
		{ID: "Exec", Action: "exec_component", ResourceType: "component", Category: CategoryManagement, NotInOpenAPISpec: true},
	}

	patternMap, err := BuildPatternMap(ops, swagger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patternMap) != 1 {
		t.Fatalf("len(patternMap) = %d, want 1 (the NotInOpenAPISpec operation must be skipped)", len(patternMap))
	}
	if _, ok := patternMap[testProjectPattern]; !ok {
		t.Fatal(`patternMap["POST /projects"] missing`)
	}
	for pattern, op := range patternMap {
		if op.ID == "Exec" {
			t.Errorf("BuildPatternMap() included the NotInOpenAPISpec operation %q under pattern %q", op.ID, pattern)
		}
	}
}
