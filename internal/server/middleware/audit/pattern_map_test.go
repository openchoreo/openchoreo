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
		{ID: testProjectOpID, Action: "create_project", ResourceType: "projects", Category: CategoryManagement},
		{ID: "UpdateProject", Action: "update_project", ResourceType: "projects", Category: CategoryManagement},
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

// TestBuildPatternMap_PatternCollision covers the realistic way this fires:
// a caller's own ops table lists the same operationId twice (e.g. a
// copy-pasted OperationDef entry with a forgotten ID change), so both
// resolve to the same spec route.
func TestBuildPatternMap_PatternCollision(t *testing.T) {
	swagger := loadTestSpec(t)
	ops := []Operation{
		{ID: "DeleteProject", Action: "delete_project", ResourceType: "projects", Category: CategoryManagement},
		{ID: "DeleteProject", Action: "delete_project_dup", ResourceType: "projects", Category: CategoryManagement},
	}

	_, err := BuildPatternMap(ops, swagger)
	if err == nil {
		t.Fatal("expected a pattern collision error, got nil")
	}
	if !strings.Contains(err.Error(), "DELETE /projects/{name}") {
		t.Errorf("error = %q, want it to name the colliding pattern", err.Error())
	}
}
