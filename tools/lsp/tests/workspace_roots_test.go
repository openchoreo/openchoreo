// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

func TestNormalizeWorkspaceRootsKeepsSiblings(t *testing.T) {
	roots := lsp.NormalizeWorkspaceRoots([]string{"/a/proj-one", "/b/proj-two"})
	want := []string{"/a/proj-one", "/b/proj-two"}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("roots = %v, want %v", roots, want)
	}
}

func TestNormalizeWorkspaceRootsDropsNestedRoot(t *testing.T) {
	roots := lsp.NormalizeWorkspaceRoots([]string{testValueAProj, "/a/proj/sub"})
	if len(roots) != 1 || roots[0] != testValueAProj {
		t.Fatalf("roots = %v, want [%s]", roots, testValueAProj)
	}
}

func TestNormalizeWorkspaceRootsShallowAfterDeep(t *testing.T) {
	// A shallower root arriving after a deeper one replaces it.
	roots := lsp.NormalizeWorkspaceRoots([]string{"/a/proj/sub", testValueAProj})
	if len(roots) != 1 || roots[0] != testValueAProj {
		t.Fatalf("roots = %v, want [%s]", roots, testValueAProj)
	}
}

func TestNormalizeWorkspaceRootsDedups(t *testing.T) {
	roots := lsp.NormalizeWorkspaceRoots([]string{testValueAProj, testValueAProj, "/a/proj/"})
	if len(roots) != 1 || roots[0] != testValueAProj {
		t.Fatalf("roots = %v, want [%s]", roots, testValueAProj)
	}
}

func TestNormalizeWorkspaceRootsIgnoresEmpty(t *testing.T) {
	roots := lsp.NormalizeWorkspaceRoots([]string{"", testValueAProj})
	if len(roots) != 1 || roots[0] != testValueAProj {
		t.Fatalf("roots = %v, want [%s]", roots, testValueAProj)
	}
}

func TestNormalizeWorkspaceRootsPrefixNameNotSubpath(t *testing.T) {
	// /a/beta shares a name prefix with /a/b but is not inside it.
	roots := lsp.NormalizeWorkspaceRoots([]string{"/a/b", "/a/beta"})
	if len(roots) != 2 {
		t.Fatalf("roots = %v, want 2 roots", roots)
	}
}

func TestNormalizeWorkspaceRootsAbsoluteFromRelative(t *testing.T) {
	roots := lsp.NormalizeWorkspaceRoots([]string{"relative/dir"})
	abs, _ := filepath.Abs("relative/dir")
	if len(roots) != 1 || roots[0] != abs {
		t.Fatalf("roots = %v, want [%s]", roots, abs)
	}
}
