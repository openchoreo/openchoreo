// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"path/filepath"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// resolveWorkspaceRoots extracts the working roots from an initialize request.
// Modern multi-root clients (e.g. VS Code multi-root workspaces) send
// workspaceFolders with rootUri left null; legacy single-folder clients send
// only rootUri. Returns the normalized, non-overlapping roots.
func resolveWorkspaceRoots(params *protocol.InitializeParams) []string {
	if params == nil {
		return nil
	}

	var roots []string
	if len(params.WorkspaceFolders) > 0 {
		for _, folder := range params.WorkspaceFolders {
			if path := fileURIToPath(folder.URI); path != "" {
				roots = append(roots, path)
			}
		}
	} else if params.RootURI != nil {
		if path := fileURIToPath(*params.RootURI); path != "" {
			roots = []string{path}
		}
	}

	return NormalizeWorkspaceRoots(roots)
}

// NormalizeWorkspaceRoots cleans and deduplicates workspace roots: paths are
// made absolute, exact duplicates are removed, and any root that is fully
// contained inside another root is dropped so files are never scanned or
// watched twice.
//
// Exported for the tests in internal/lsp/tests.
func NormalizeWorkspaceRoots(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		abs = filepath.Clean(abs)
		if abs != string(filepath.Separator) {
			abs = strings.TrimRight(abs, string(filepath.Separator))
		}

		duplicate := false
		for _, existing := range out {
			if existing == abs {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}

		// Skip abs if it is contained in an already-accepted root.
		contained := false
		for _, existing := range out {
			if isSubpath(abs, existing) {
				contained = true
				break
			}
		}
		if contained {
			continue
		}

		// Drop existing roots now contained by the new, deeper abs.
		kept := out[:0]
		for _, existing := range out {
			if !isSubpath(existing, abs) {
				kept = append(kept, existing)
			}
		}
		out = kept
		out = append(out, abs)
	}
	return out
}

// isSubpath reports whether child lies inside parent (or equals it). The
// ordering matters for the leaf-of-path check: a directory that merely shares
// a name prefix ("/a/b" vs "/a/beta") must not count as a subpath.
func isSubpath(child, parent string) bool {
	if child == parent {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
