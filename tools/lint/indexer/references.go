// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package indexer

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// SnapshotDocument retains parsed source identity for workspace consumers.
type SnapshotDocument struct {
	URI      string
	Document *parser.DocumentNode
}

func uriPath(uri string) string {
	u, e := url.Parse(uri)
	if e == nil && u.Scheme == "file" {
		return filepath.FromSlash(u.Path)
	}
	return uri
}
func within(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Snapshot uses the shared index's files and latest UpdateFile content. Invalid
// editor buffers replace disk content rather than resurrecting stale definitions.
func (idx *Indexer) Snapshot(sourceURI string) []SnapshotDocument {
	idx.mu.RLock()
	roots := append([]string(nil), idx.rootDirs...)
	version := idx.version
	if idx.snapshotVersion == version {
		cached := append([]SnapshotDocument(nil), idx.snapshotDocuments...)
		idx.mu.RUnlock()
		return scopeDocuments(cached, roots, sourceURI)
	}
	files := map[string][]byte{}
	for uri, data := range idx.contents {
		files[uri] = append([]byte(nil), data...)
	}
	idx.mu.RUnlock()
	var out []SnapshotDocument
	for uri, data := range files {
		docs, err := parser.ParseYAML(data)
		if err != nil {
			continue
		}
		for _, d := range docs {
			if parser.IsOpenChoreoAPIVersion(d.APIVersion) {
				out = append(out, SnapshotDocument{URI: uri, Document: d})
			}
		}
	}
	idx.mu.Lock()
	if idx.version == version {
		idx.snapshotVersion = version
		idx.snapshotDocuments = out
	}
	idx.mu.Unlock()
	return scopeDocuments(out, roots, sourceURI)
}
func scopeDocuments(docs []SnapshotDocument, roots []string, sourceURI string) []SnapshotDocument {
	root := ""
	source := uriPath(sourceURI)
	for _, r := range roots {
		if within(r, source) && len(r) > len(root) {
			root = r
		}
	}
	if root == "" && len(roots) > 0 {
		return nil
	}
	var out []SnapshotDocument
	for _, d := range docs {
		if root == "" || within(root, uriPath(d.URI)) {
			out = append(out, d)
		}
	}
	return out
}

func ReferenceName(kind, name string) string {
	if kind == "ComponentType" || kind == "ClusterComponentType" {
		if i := strings.LastIndex(name, "/"); i >= 0 {
			return name[i+1:]
		}
	}
	return name
}
func (idx *Indexer) FindScopedDefinition(kind, namespace, name, sourceURI string) []Location {
	if namespace == "" {
		namespace = "default"
	}
	normalized := ReferenceName(kind, name)
	var result []Location
	for _, entry := range idx.Snapshot(sourceURI) {
		d := entry.Document
		if d.Kind != kind || d.Metadata.Name != normalized {
			continue
		}
		if !strings.HasPrefix(kind, "Cluster") && NamespaceOrDefault(d.Metadata.Namespace) != namespace {
			continue
		}
		if normalized != name {
			prefix, _, _ := strings.Cut(name, "/")
			match := false
			if d.Spec != nil {
				for i := 0; i+1 < len(d.Spec.Content); i += 2 {
					if d.Spec.Content[i].Value == "workloadType" && d.Spec.Content[i+1].Value == prefix {
						match = true
					}
				}
			}
			if !match {
				continue
			}
		}
		result = append(result, Location{URI: entry.URI, Line: d.Metadata.Node.Line, Column: d.Metadata.Node.Column})
	}
	return result
}

func NamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return "default"
	}
	return namespace
}
func (idx *Indexer) HasWorkspace(sourceURI string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, root := range idx.rootDirs {
		if within(root, uriPath(sourceURI)) {
			return true
		}
	}
	return false
}

// SchemaSources returns immutable source copies for schema dependencies without
// changing Snapshot's OpenChoreo-only navigation and reference behavior.
func (idx *Indexer) SchemaSources(sourceURI string) map[string][]byte {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	root := ""
	source := uriPath(sourceURI)
	for _, candidate := range idx.rootDirs {
		if within(candidate, source) && len(candidate) > len(root) {
			root = candidate
		}
	}
	out := map[string][]byte{}
	if root == "" && len(idx.rootDirs) > 0 {
		return out
	}
	for uri, data := range idx.contents {
		path := uriPath(uri)
		if root == "" || within(root, path) {
			out[path] = append([]byte(nil), data...)
		}
	}
	return out
}

// SourceContent returns a copy of the indexed source, including malformed YAML.
func (idx *Indexer) SourceContent(uri string) ([]byte, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	data, ok := idx.contents[uri]
	return append([]byte(nil), data...), ok
}
