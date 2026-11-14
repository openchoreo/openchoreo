// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

type schemaScopeEntry struct {
	revision uint64
	scope    *targetschema.Scope
}

func (s *Server) targetSchemaScope(uri string, overrides map[string][]byte) *targetschema.Scope {
	root := s.postRenderRoot(fileURIToPath(uri))
	revision := s.indexer.Version()
	cacheable := true
	for path, data := range overrides {
		if strings.Contains(string(data), "CustomResourceDefinition") || s.isSchemaDependency(path) {
			cacheable = false
			break
		}
	}
	s.schemaMu.Lock()
	defer s.schemaMu.Unlock()
	if cacheable {
		if entry, ok := s.schemaScopes[root]; ok && entry.revision == revision {
			return entry.scope
		}
	}
	sources := s.indexer.SchemaSources(uri)
	for _, doc := range s.snapshotOpenDocuments() {
		if p := fileURIToPath(doc.URI); p != "" {
			sources[p] = []byte(doc.Text)
		}
	}
	for path, data := range overrides {
		sources[path] = data
	}
	scope := targetschema.NewScope(root, sources)
	s.watchSchemaDependencies(scope.Dependencies())
	scope.ReleaseSources()
	if cacheable {
		if s.schemaScopes == nil {
			s.schemaScopes = map[string]schemaScopeEntry{}
		}
		s.schemaScopes[root] = schemaScopeEntry{revision, scope}
	}
	return scope
}

func (s *Server) freshTargetSchemaScope(uri string, overrides map[string][]byte) *targetschema.Scope {
	root := s.postRenderRoot(fileURIToPath(uri))
	sources, err := targetschema.ReadSources(root, true)
	if err != nil {
		sources = s.indexer.SchemaSources(uri)
	}
	for _, doc := range s.snapshotOpenDocuments() {
		if p := fileURIToPath(doc.URI); p != "" {
			sources[p] = []byte(doc.Text)
		}
	}
	for path, data := range overrides {
		sources[path] = data
	}
	scope := targetschema.NewScope(root, sources)
	s.watchSchemaDependencies(scope.Dependencies())
	return scope
}

func (s *Server) watchSchemaDependencies(paths []string) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.schemaDependencyPaths == nil {
		s.schemaDependencyPaths = map[string]struct{}{}
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		s.schemaDependencyPaths[path] = struct{}{}
		if s.watcher != nil {
			_ = s.watcher.Add(filepath.Dir(path))
		}
	}
}
func (s *Server) isSchemaDependency(path string) bool {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	_, ok := s.schemaDependencyPaths[filepath.Clean(path)]
	return ok
}

func (s *Server) bindTargetSchemas(uri, text string, docs []*parser.DocumentNode) []*parser.DocumentNode {
	scope := s.targetSchemaScope(uri, map[string][]byte{fileURIToPath(uri): []byte(text)})
	copies := make([]*parser.DocumentNode, 0, len(docs))
	for _, doc := range docs {
		copyDoc := *doc
		copyDoc.ValidationContext = targetschema.WithResolver(doc.ValidationContext, scope)
		copies = append(copies, &copyDoc)
	}
	return copies
}
