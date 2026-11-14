// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"path/filepath"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) workspaceRootsSnapshot() []string {
	s.workspaceRootsMu.RLock()
	defer s.workspaceRootsMu.RUnlock()
	return append([]string(nil), s.workspaceRoots...)
}

// DidChangeWorkspaceFolders rescans asynchronously; handlers and completion stay responsive.
func (s *Server) DidChangeWorkspaceFolders(_ *glsp.Context, params *protocol.DidChangeWorkspaceFoldersParams) error {
	s.workspaceRootsMu.Lock()
	roots := append([]string(nil), s.workspaceRoots...)
	for _, removed := range params.Event.Removed {
		path := filepath.Clean(fileURIToPath(removed.URI))
		var kept []string
		for _, root := range roots {
			if root != path {
				kept = append(kept, root)
			}
		}
		roots = kept
	}
	for _, added := range params.Event.Added {
		roots = append(roots, fileURIToPath(added.URI))
	}
	s.workspaceRoots = NormalizeWorkspaceRoots(roots)
	s.workspaceRootsMu.Unlock()
	go s.refreshWorkspaceFolders()
	return nil
}

func (s *Server) refreshWorkspaceFolders() {
	if s.initialIndexStarted {
		<-s.indexDone
	}
	s.workspaceChangeMu.Lock()
	defer s.workspaceChangeMu.Unlock()
	s.preRenderRunMu.Lock()
	roots := s.workspaceRootsSnapshot()
	s.StopWatcher()
	_ = s.indexer.IndexWorkspaces(roots)
	for _, doc := range s.snapshotOpenDocuments() {
		for _, root := range roots {
			if projectContains(root, fileURIToPath(doc.URI)) {
				_ = s.indexer.UpdateFile(doc.URI, []byte(doc.Text))
				break
			}
		}
	}
	s.invalidateValidationCache()
	var removed []string
	s.preRenderSchemaMu.Lock()
	for uri := range s.workspaceStaticByURI {
		belongs := false
		for _, root := range roots {
			if projectContains(root, fileURIToPath(uri)) {
				belongs = true
				break
			}
		}
		if !belongs {
			delete(s.workspaceStaticByURI, uri)
			delete(s.preRenderSchemaByURI, uri)
			delete(s.preRenderSchemaFiles, fileURIToPath(uri))
			removed = append(removed, uri)
		}
	}
	s.preRenderSchemaMu.Unlock()
	s.watchMu.Lock()
	s.watchClosed = false
	s.watchMu.Unlock()
	s.startWatch(roots)
	s.preRenderRunMu.Unlock()
	for _, uri := range removed {
		s.invalidatePostRenderForEdit(uri)
		s.publishDiagnosticsAsync(uri, 0, nil)
	}
	for _, root := range roots {
		s.triggerPreRenderSchemaValidationForPath(filepath.Join(root, ".workspace-diagnostics.yaml"))
	}
}
