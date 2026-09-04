// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// writeMethodPrefixes names the method-name prefixes this package uses for
// handlers that persist something. The MCP audit binding table keys operations
// by tool name rather than by handler method (see the audit package's
// mcpEnrichment), and the two names diverge often enough — GenerateRelease is
// served by CreateComponentRelease, UpdateComponent by PatchComponent — that
// deriving the set of write handlers from the bindings isn't possible
// statically. Matching on the prefix this package actually uses is the
// workable rule; extraWriteMethods and wantCheckedWriteMethods together cover
// what a prefix rule alone would miss.
var writeMethodPrefixes = []string{"Create", "Update", "Patch"}

// extraWriteMethods names write handlers whose method name carries none of
// writeMethodPrefixes.
var extraWriteMethods = map[string]bool{
	"TriggerWorkflowRun": true, // creates a WorkflowRun; alias entry point of CreateWorkflowRun
}

// setResourceExemptions maps a write handler's method name to the reason it
// records no resource identity. Empty today: every write handler in this
// package gets the persisted object back from its service call. A future
// handler that genuinely cannot (a fire-and-forget write, a service returning
// only an error) belongs here with a reason, not silently outside the rule.
var setResourceExemptions = map[string]string{}

// wantCheckedWriteMethods pins how many write handlers the rule below actually
// checked, and wantHandlerMethods how many (*MCPHandler) methods exist at all.
// Both are needed, and the second is the load-bearing one: a future mutating
// handler named outside writeMethodPrefixes — a PromoteRelease, a
// RollbackBinding — leaves the checked count at exactly its old value and would
// pass this file by in silence. Pinning the total is what forces a look, since
// no method can be added without moving it. Pinned the way
// audit_coverage_test.go pins its tool and binding totals.
//
// On a mismatch: if the new method writes, give it a Create/Update/Patch name
// or add it to extraWriteMethods; if it only reads, just update the total.
const (
	wantCheckedWriteMethods = 54
	wantHandlerMethods      = 170
)

// TestMCPHandlers_WritesCallSetResource statically guards the UID gap on the
// MCP surface, mirroring api/handlers' TestHandlers_BodyCarriedCreatesCallSetResource
// and TestHandlers_UpdatesCallSetResource on REST.
//
// The MCP audit middleware seeds resource.name and resource.namespace from a
// tools/call's raw arguments before the handler runs, which is what covers a
// denied or failed call — but nothing in the call can carry the object's UID,
// and for a create whose binding has no ResourceArg (create_workload,
// create_release_binding, create_workflow_run and the trigger_workflow_run
// alias) nothing carries the resource's name either. Both come only from the
// handler recording the persisted object. Without this test a write handler can
// be added, or an existing one edited, with that call missing: the tool still
// resolves through the audit middleware and still succeeds, it just audits
// UID-less — and on those four, nameless — forever, with nothing before this
// point catching it.
func TestMCPHandlers_WritesCallSetResource(t *testing.T) {
	methods, err := parseMCPHandlerMethods(t)
	if err != nil {
		t.Fatalf("failed to parse MCP handler methods: %v", err)
	}

	checked := 0
	for name, fn := range methods {
		if !isWriteMethod(name) {
			continue
		}
		checked++
		if reason, exempted := setResourceExemptions[name]; exempted {
			if reason == "" {
				t.Errorf("setResourceExemptions[%q] has an empty reason", name)
			}
			continue
		}
		if !callsSetAuditResource(fn) {
			t.Errorf("(*MCPHandler) %s records no audit resource — a handler that creates or updates "+
				"an object must call setAuditResource(ctx, obj) (or audit.SetResource directly) once the "+
				"write succeeds, since the tool call's arguments can never carry the object's UID", name)
		}
	}

	if checked != wantCheckedWriteMethods {
		t.Errorf("checked %d write handlers, want %d — a write handler was added or removed; if its name "+
			"matches none of %v, add it to extraWriteMethods so it is not skipped silently",
			checked, wantCheckedWriteMethods, writeMethodPrefixes)
	}
	if len(methods) != wantHandlerMethods {
		t.Errorf("found %d (*MCPHandler) methods, want %d — a handler was added or removed. If it writes, "+
			"make sure the rule above covers it (a name matching %v, or an extraWriteMethods entry) rather "+
			"than only updating this number; a mutating handler outside that rule audits with no resource "+
			"id, and nothing else would catch it", len(methods), wantHandlerMethods, writeMethodPrefixes)
	}

	for name := range setResourceExemptions {
		if _, ok := methods[name]; !ok {
			t.Errorf("setResourceExemptions names %q, which is not a (*MCPHandler) method "+
				"(renamed or removed handler?)", name)
		}
	}
	for name := range extraWriteMethods {
		if _, ok := methods[name]; !ok {
			t.Errorf("extraWriteMethods names %q, which is not a (*MCPHandler) method "+
				"(renamed or removed handler?)", name)
		}
	}
}

// isWriteMethod reports whether a method name identifies a handler that
// persists something.
func isWriteMethod(name string) bool {
	if extraWriteMethods[name] {
		return true
	}
	for _, prefix := range writeMethodPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// parseMCPHandlerMethods parses every non-test .go file in this package
// directory and returns every top-level method declared on a *MCPHandler
// receiver, keyed by method name.
func parseMCPHandlerMethods(t *testing.T) (map[string]*ast.FuncDecl, error) {
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
			if mcpReceiverTypeName(fn.Recv.List[0].Type) == "MCPHandler" {
				methods[fn.Name.Name] = fn
			}
		}
	}
	return methods, nil
}

// mcpReceiverTypeName returns the bare type name of a (possibly pointer)
// receiver expression, e.g. "MCPHandler" for both "MCPHandler" and
// "*MCPHandler".
func mcpReceiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// callsSetAuditResource reports whether fn's body contains a call shaped like
// setAuditResource(...) or audit.SetResource(...) anywhere in its syntax tree,
// including inside nested blocks, closures, and branches. Both forms count:
// setAuditResource is this package's helper for the common case where the
// persisted object supplies every field, and a direct audit.SetResource call
// stays valid for a handler that has to assemble the Resource by hand (the
// delete handlers do, having no object left to read).
func callsSetAuditResource(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "setAuditResource" {
				found = true
				return false
			}
		case *ast.SelectorExpr:
			if fun.Sel.Name != "SetResource" {
				return true
			}
			if pkgIdent, ok := fun.X.(*ast.Ident); ok && pkgIdent.Name == "audit" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
