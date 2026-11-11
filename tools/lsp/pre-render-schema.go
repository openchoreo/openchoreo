// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	prerendervalidator "github.com/openchoreo/openchoreo/tools/lint/pre-render-validator"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const preRenderPreRenderSchemaValidationSource = "openchoreo-schema"

// runPreRenderSchemaValidationFiles validates Component/trait parameters against schemas
// for the given files and publishes diagnostics.
func (s *Server) runPreRenderSchemaValidationFiles(files []string) {
	s.preRenderRunMu.Lock()
	defer s.preRenderRunMu.Unlock()
	revision := s.indexer.Version()
	overlays := make(map[string]string)
	for _, doc := range s.snapshotOpenDocuments() {
		overlays[filepath.Clean(fileURIToPath(doc.URI))] = doc.Text
	}
	indexed := map[string][]*parser.DocumentNode{}
	if len(files) > 0 {
		for _, entry := range s.indexer.Snapshot(pathToDocumentURI(files[0])) {
			p := filepath.Clean(fileURIToPath(entry.URI))
			indexed[p] = append(indexed[p], entry.Document)
		}
	}
	var resources []prerendervalidator.Resource
	seen := make(map[string]bool)
	for _, path := range files {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		if _, open := overlays[path]; !open && len(indexed[path]) > 0 {
			for _, doc := range indexed[path] {
				resources = append(resources, prerendervalidator.Resource{Path: path, Document: doc})
			}
			continue
		}
		text, open := overlays[path]
		if !open {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			text = string(data)
		}
		docs, err := parser.ParseYAML([]byte(text))
		if err != nil {
			continue
		}
		for _, doc := range docs {
			resources = append(resources, prerendervalidator.Resource{Path: path, Document: doc})
		}
	}
	if len(files) > 0 {
		seenDocs := map[string]bool{}
		for _, r := range resources {
			seenDocs[fmt.Sprintf("%s:%d", filepath.Clean(r.Path), r.Document.DocumentIndex)] = true
		}
		for _, entry := range s.indexer.Snapshot(pathToDocumentURI(files[0])) {
			p := filepath.Clean(fileURIToPath(entry.URI))
			if _, open := overlays[p]; open {
				continue
			}
			key := fmt.Sprintf("%s:%d", p, entry.Document.DocumentIndex)
			if !seenDocs[key] {
				resources = append(resources, prerendervalidator.Resource{Path: p, Document: entry.Document})
			}
		}
	}
	// Validate unopened files through the same cached static entrypoint used by editors.
	static := make(map[string][]protocol.Diagnostic)
	rawSources := map[string][]byte{}
	if len(files) > 0 {
		rawSources = s.indexer.SchemaSources(pathToDocumentURI(files[0]))
	}
	for path := range seen {
		uri := pathToDocumentURI(path)
		text, open := overlays[path]
		if !open {
			data, ok := rawSources[path]
			var err error
			if !ok {
				data, err = os.ReadFile(path)
			}
			if err != nil {
				s.deleteValidationEntry(uri)
				static[uri] = nil
				continue
			}
			text = string(data)
		}
		static[uri] = s.diagnosticsForCached(uri, text)
	}
	findings := prerendervalidator.ValidateDocuments(context.Background(), resources)
	if s.indexer.Version() != revision {
		return
	}
	s.preRenderSchemaMu.Lock()
	if s.workspaceStaticByURI == nil {
		s.workspaceStaticByURI = make(map[string][]protocol.Diagnostic)
	}
	for uri, diags := range static {
		s.workspaceStaticByURI[uri] = diags
	}
	s.preRenderSchemaMu.Unlock()
	if len(findings) == 0 {
		s.clearSchemaFindings(files)
		return
	}

	byURI := make(map[string][]protocol.Diagnostic)
	for _, finding := range findings {
		uri := pathToDocumentURI(finding.Path)
		byURI[uri] = append(byURI[uri], schemaFindingToDiagnostic(finding))
	}

	s.preRenderSchemaMu.Lock()
	affected := make(map[string]struct{})
	if s.preRenderSchemaByURI == nil {
		s.preRenderSchemaByURI = make(map[string][]protocol.Diagnostic)
	}
	if s.preRenderSchemaFiles == nil {
		s.preRenderSchemaFiles = make(map[string]struct{})
	}
	for _, f := range files {
		uri := pathToDocumentURI(f)
		if _, hadSchema := s.preRenderSchemaByURI[uri]; hadSchema || s.GetDocument(uri) == nil {
			affected[uri] = struct{}{}
		}
		delete(s.preRenderSchemaByURI, uri)
		s.preRenderSchemaFiles[filepath.Clean(f)] = struct{}{}
	}
	for uri, diags := range byURI {
		s.preRenderSchemaByURI[uri] = diags
		affected[uri] = struct{}{}
	}
	s.preRenderSchemaMu.Unlock()

	open := make(map[string]Document)
	for _, doc := range s.snapshotOpenDocuments() {
		open[doc.URI] = doc
	}
	for uri := range affected {
		if doc, ok := open[uri]; ok {
			structural := s.diagnosticsForCached(uri, doc.Text)
			s.publishDiagnosticsAsync(uri, doc.Version, structural)
		} else {
			s.publishDiagnosticsAsync(uri, 0, nil)
		}
	}
}

func (s *Server) clearSchemaFindings(files []string) {
	if len(files) == 0 {
		return
	}
	s.preRenderSchemaMu.Lock()
	affected := make(map[string]struct{})
	for _, f := range files {
		uri := pathToDocumentURI(f)
		if _, hadSchema := s.preRenderSchemaByURI[uri]; hadSchema || s.GetDocument(uri) == nil {
			affected[uri] = struct{}{}
		}
		delete(s.preRenderSchemaByURI, uri)
		delete(s.preRenderSchemaFiles, filepath.Clean(f))
	}
	s.preRenderSchemaMu.Unlock()

	open := make(map[string]Document)
	for _, doc := range s.snapshotOpenDocuments() {
		open[doc.URI] = doc
	}
	for uri := range affected {
		if doc, ok := open[uri]; ok {
			structural := s.diagnosticsForCached(uri, doc.Text)
			s.publishDiagnosticsAsync(uri, doc.Version, structural)
		} else {
			s.publishDiagnosticsAsync(uri, 0, nil)
		}
	}
}

func schemaFindingToDiagnostic(f prerendervalidator.Finding) protocol.Diagnostic {
	line := f.Line
	column := f.Column
	if line < 1 {
		line = 1
	}
	if column < 1 {
		column = 1
	}
	start := protocol.Position{
		Line:      protocol.UInteger(line - 1),
		Character: protocol.UInteger(column - 1),
	}
	end := protocol.Position{
		Line:      start.Line,
		Character: start.Character + 1,
	}
	severity := protocol.DiagnosticSeverityError
	switch f.Severity {
	case prerendervalidator.SeverityWarning:
		severity = protocol.DiagnosticSeverityWarning
	case prerendervalidator.SeverityInfo:
		severity = protocol.DiagnosticSeverityInformation
	case prerendervalidator.SeverityHint:
		severity = protocol.DiagnosticSeverityHint
	}
	return protocol.Diagnostic{
		Range:    protocol.Range{Start: start, End: end},
		Severity: &severity,
		Code:     &protocol.IntegerOrString{Value: f.Code},
		Source:   stringPtr(preRenderPreRenderSchemaValidationSource),
		Message:  f.Message,
	}
}

func (s *Server) mergeSchemaDiagnostics(uri string, diags []protocol.Diagnostic) []protocol.Diagnostic {
	s.preRenderSchemaMu.Lock()
	cached := append([]protocol.Diagnostic(nil), s.preRenderSchemaByURI[uri]...)
	s.preRenderSchemaMu.Unlock()
	if len(cached) == 0 {
		return diags
	}
	merged := make([]protocol.Diagnostic, 0, len(diags)+len(cached))
	merged = append(merged, diags...)
	merged = append(merged, cached...)
	return merged
}

func (s *Server) triggerPreRenderSchemaValidationForPath(path string) {
	dir := filepath.Dir(path)
	if len(s.workspaceRootsSnapshot()) > 0 {
		contained := false
		for _, root := range s.workspaceRootsSnapshot() {
			if projectContains(root, path) {
				contained = true
				break
			}
		}
		if !contained {
			if s.isSchemaDependency(path) {
				for _, root := range s.workspaceRootsSnapshot() {
					s.triggerPreRenderSchemaValidationForPath(filepath.Join(root, ".workspace-diagnostics.yaml"))
				}
			}
			return
		}
	}
	for _, root := range s.workspaceRootsSnapshot() {
		if projectContains(root, path) {
			dir = root
			break
		}
	}

	// Use the containing workspace so sibling platform definitions are available.
	files, err := parser.CollectYAMLFiles(dir, true)
	if err != nil {
		return
	}
	if _, err := os.Stat(path); err == nil {
		// ensure path included
		found := false
		for _, f := range files {
			if filepath.Clean(f) == filepath.Clean(path) {
				found = true
				break
			}
		}
		if !found {
			files = append(files, path)
		}
	}
	for _, doc := range s.snapshotOpenDocuments() {
		p := fileURIToPath(doc.URI)
		rel, e := filepath.Rel(dir, p)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			files = append(files, p)
		}
	}
	// Include the changed path even after deletion so cached findings clear.
	files = append(files, path)
	s.preRenderSchemaMu.Lock()
	for prior := range s.preRenderSchemaFiles {
		rel, e := filepath.Rel(dir, prior)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			files = append(files, prior)
		}
	}
	s.preRenderSchemaMu.Unlock()
	clean := files[:0]
	for _, file := range files {
		if filepath.Base(file) != ".workspace-diagnostics.yaml" {
			clean = append(clean, file)
		}
	}
	s.runPreRenderSchemaValidationFiles(clean)
}

func (s *Server) mergePreRenderSchemaDiagnostics(uri string, diags []protocol.Diagnostic) []protocol.Diagnostic {
	s.preRenderSchemaMu.Lock()
	cached := append([]protocol.Diagnostic(nil), s.preRenderSchemaByURI[uri]...)
	s.preRenderSchemaMu.Unlock()
	if len(cached) == 0 {
		return diags
	}
	merged := make([]protocol.Diagnostic, 0, len(diags)+len(cached))
	merged = append(merged, diags...)
	for _, finding := range cached {
		duplicate := false
		if finding.Code != nil && finding.Code.Value == "pre-render-validation-failed" {
			for _, prior := range diags {
				if prior.Source != nil && *prior.Source == "openchoreo-post-render" && strings.Contains(prior.Message, finding.Message) {
					duplicate = true
					break
				}
			}
		}
		if !duplicate {
			merged = append(merged, finding)
		}
	}
	return merged
}
