// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
)

// TestHandlers_BodyCarriedCreatesCallSetResource statically guards a fact
// that no runtime test can: for an operation with no RESTResourceParam, the
// audit envelope's Resource identity comes entirely from a handler calling
// audit.SetResource — there is no path parameter to fall back on. If a
// future handler for one of these operations is written (or edited) without
// that call, the operation still resolves through the audit middleware and
// returns 2xx; it just silently audits with an empty Resource on every
// success, and nothing before this test would catch that.
//
// Scope: only operations whose OperationID names a *Handler method
// (generatedOperationDefs, i.e. NotInOpenAPISpec is false) are checked here.
// Exec and Wirelogs are body-less, non-OpenAPI routes served by ServeHTTP on
// their own handler types rather than an OperationID-named method, so they
// don't fit this AST lookup; both already call audit.SetResource (see
// exec.go, wirelogs.go) and are covered by exec_wirelogs_audit_test.go
// instead.
func TestHandlers_BodyCarriedCreatesCallSetResource(t *testing.T) {
	methods, err := parseHandlerMethods(t)
	if err != nil {
		t.Fatalf("failed to parse handler methods: %v", err)
	}

	for _, op := range apiaudit.GetOperations() {
		if op.RESTResourceParam != "" || op.NotInOpenAPISpec {
			continue
		}

		fn, ok := methods[op.ID]
		if !ok {
			t.Errorf("operation %q has no RESTResourceParam but no (*Handler) %s method was found "+
				"in internal/openchoreo-api/api/handlers", op.ID, op.ID)
			continue
		}

		if !callsAuditSetResource(fn) {
			t.Errorf("(*Handler) %s (operation %q) has no RESTResourceParam, so the audit envelope's "+
				"Resource must come from an audit.SetResource(ctx, ...) call in its body — none was found",
				op.ID, op.ID)
		}
	}
}

// TestHandlers_UpdatesCallSetResource statically guards the UID gap across
// every update_* handler: RESTResourceParam already gives resource.name even
// when a request is denied or fails before the handler runs, but the
// resource's UID is only ever known once the update actually succeeds — the
// handler has to call audit.SetResource itself to add it. Without this
// test, an update handler can be added (or edited) without that call, and
// its audit events would silently carry no resource.id. This is the mirror
// of TestHandlers_BodyCarriedCreatesCallSetResource for the update_* verb
// rather than the no-RESTResourceParam shape, so such a handler fails here
// instead of shipping silently UID-less.
func TestHandlers_UpdatesCallSetResource(t *testing.T) {
	methods, err := parseHandlerMethods(t)
	if err != nil {
		t.Fatalf("failed to parse handler methods: %v", err)
	}

	for _, op := range apiaudit.GetOperations() {
		if !strings.HasPrefix(op.Action, "update_") || op.NotInOpenAPISpec {
			continue
		}

		fn, ok := methods[op.ID]
		if !ok {
			t.Errorf("operation %q (action %q) has no (*Handler) %s method found "+
				"in internal/openchoreo-api/api/handlers", op.ID, op.Action, op.ID)
			continue
		}

		if !callsAuditSetResource(fn) {
			t.Errorf("(*Handler) %s (operation %q) has no audit.SetResource(ctx, ...) call in its body — "+
				"an update handler must add the resource's UID once the update succeeds, since "+
				"RESTResourceParam alone only ever gives resource.name", op.ID, op.ID)
		}
	}
}

// parseHandlerMethods parses every non-test .go file in this package
// directory and returns every top-level method declared on a *Handler
// receiver, keyed by method name.
func parseHandlerMethods(t *testing.T) (map[string]*ast.FuncDecl, error) {
	t.Helper()

	paths, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	methods := make(map[string]*ast.FuncDecl)
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if receiverTypeName(fn.Recv.List[0].Type) == "Handler" {
				methods[fn.Name.Name] = fn
			}
		}
	}
	return methods, nil
}

// receiverTypeName returns the bare type name of a (possibly pointer)
// receiver expression, e.g. "Handler" for both "Handler" and "*Handler".
func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// callsAuditSetResource reports whether fn's body contains a call shaped
// like audit.SetResource(...) anywhere in its syntax tree, including inside
// nested blocks, closures, and branches.
func callsAuditSetResource(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SetResource" {
			return true
		}
		if pkgIdent, ok := sel.X.(*ast.Ident); ok && pkgIdent.Name == "audit" {
			found = true
			return false
		}
		return true
	})
	return found
}
