// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	postrendervalidator "github.com/openchoreo/openchoreo/tools/lint/post-render-validator"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const (
	postRenderCommand = "openchoreo.runPostRenderValidation"
	postRenderSource  = "openchoreo-post-render"
	postRenderTimeout = 60 * time.Second
)

// postRenderResult is the JSON payload returned by workspace/executeCommand.
// The extension uses it to build the progress notification and to decide
// whether to offer the Problems panel.
type postRenderResult struct {
	Files    int      `json:"files"`
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
	Message  string   `json:"message,omitempty"`
	Details  []string `json:"details,omitempty"`
}

// CodeLens offers the post-render action at the top of OpenChoreo YAML files.
func (s *Server) CodeLens(
	context *glsp.Context,
	params *protocol.CodeLensParams,
) ([]protocol.CodeLens, error) {
	if params == nil {
		return nil, fmt.Errorf("missing codeLens parameters")
	}
	uri := string(params.TextDocument.URI)
	text, ok := s.documentText(uri)
	if !ok || !parser.HasOpenChoreoAPIVersion([]byte(text)) {
		return []protocol.CodeLens{}, nil
	}
	if len(uri) < len("file://") || uri[:len("file://")] != "file://" {
		return []protocol.CodeLens{}, nil
	}

	return []protocol.CodeLens{{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 0},
		},
		Command: &protocol.Command{
			Title:     "Run Post-Render Validation",
			Command:   postRenderCommand,
			Arguments: []any{uri},
		},
	}}, nil
}

// ExecuteCommand handles openchoreo.runPostRenderValidation. The command takes
// the document URI as its only argument and validates the resources in that
// file's directory.
func (s *Server) ExecuteCommand(
	context *glsp.Context,
	params *protocol.ExecuteCommandParams,
) (any, error) {
	if params == nil {
		return nil, fmt.Errorf("missing executeCommand parameters")
	}
	if params.Command != postRenderCommand {
		return nil, fmt.Errorf("unknown command %q", params.Command)
	}
	uri, err := postRenderURI(params.Arguments)
	if err != nil {
		return nil, err
	}
	return s.runPostRenderValidation(uri), nil
}

func postRenderURI(args []any) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("missing document URI for %s", postRenderCommand)
	}
	switch value := args[0].(type) {
	case string:
		if value == "" {
			return "", fmt.Errorf("empty document URI for %s", postRenderCommand)
		}
		return value, nil
	default:
		return "", fmt.Errorf("unsupported document URI argument %T", args[0])
	}
}

func (s *Server) runPostRenderValidation(uri string) postRenderResult {
	s.postRenderMu.Lock()
	if s.postRenderRunning {
		s.postRenderMu.Unlock()
		return postRenderResult{Message: "OpenChoreo post-render validation is already running."}
	}
	s.postRenderRunning = true
	epoch := s.postRenderEpoch
	s.postRenderMu.Unlock()
	defer func() {
		s.postRenderMu.Lock()
		s.postRenderRunning = false
		s.postRenderMu.Unlock()
	}()

	activePath := fileURIToPath(uri)
	if activePath == "" {
		return postRenderResult{Errors: 1, Message: "invalid document URI"}
	}

	root := s.postRenderRoot(activePath)
	files, err := parser.CollectYAMLFiles(root, true)
	if err != nil {
		return s.publishPostRenderFailure(uri, activePath, err)
	}
	files = ensureActiveFile(files, activePath)

	ctx, cancel := context.WithTimeout(context.Background(), postRenderTimeout)
	defer cancel()
	findings := postrendervalidator.ValidateProject(ctx, files, activePath)

	return s.publishPostRenderFindings(uri, activePath, files, findings, &epoch)
}

// postRenderRoot picks the directory to scan for a post-render run. A
// Component usually lives under projects/<project>/components/<name>, while the
// ComponentTypes and Environments it needs live under a sibling platform
// directory. Scanning only the active file's directory would miss them, so use
// the nearest ancestor that has both platform and projects directories. Flat
// projects without that layout keep the previous active-directory scope.
func (s *Server) postRenderRoot(activePath string) string {
	activeDir := filepath.Dir(activePath)
	workspaceRoot := ""
	for _, root := range s.workspaceRoots {
		rel, err := filepath.Rel(root, activePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(root) > len(workspaceRoot) {
			workspaceRoot = root
		}
	}

	dir := activeDir
	for {
		if isDirectory(filepath.Join(dir, "platform")) && isDirectory(filepath.Join(dir, "projects")) {
			return dir
		}
		if workspaceRoot != "" && filepath.Clean(dir) == filepath.Clean(workspaceRoot) {
			return activeDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return activeDir
		}
		dir = parent
	}
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ensureActiveFile(files []string, activePath string) []string {
	active := filepath.Clean(activePath)
	for _, file := range files {
		if filepath.Clean(file) == active {
			return files
		}
	}
	return append(files, activePath)
}

func (s *Server) publishPostRenderFailure(activeURI, activePath string, err error) postRenderResult {
	findings := []postrendervalidator.Finding{{
		Path:     activePath,
		Severity: "error",
		Code:     "post-render-error",
		Message:  err.Error(),
	}}
	return s.publishPostRenderFindings(activeURI, activePath, []string{activePath}, findings, nil)
}

// publishPostRenderFindings stores the new findings, clears findings from the
// previous run that are no longer reported, and republishes every affected
// document with structural diagnostics merged back in.
func (s *Server) publishPostRenderFindings(
	activeURI string,
	activePath string,
	files []string,
	findings []postrendervalidator.Finding,
	expectedEpoch *uint64,
) postRenderResult {
	byURI := make(map[string][]protocol.Diagnostic)
	for _, finding := range findings {
		uri := pathToDocumentURI(finding.Path)
		if samePath(finding.Path, activePath) && activeURI != "" {
			uri = activeURI
		}
		byURI[uri] = append(byURI[uri], postRenderDiagnostic(finding))
	}

	fileSet := make(map[string]struct{}, len(files))
	for _, file := range files {
		fileSet[filepath.Clean(file)] = struct{}{}
	}
	if activePath != "" {
		fileSet[filepath.Clean(activePath)] = struct{}{}
	}

	s.postRenderMu.Lock()
	if expectedEpoch != nil && *expectedEpoch != s.postRenderEpoch {
		s.postRenderMu.Unlock()
		return postRenderResult{
			Files:   len(files),
			Message: "Files changed during post-render validation; run it again.",
		}
	}
	oldByURI := s.postRenderByURI
	s.postRenderByURI = byURI
	s.postRenderFiles = fileSet
	s.postRenderMu.Unlock()

	affected := make(map[string]struct{}, len(oldByURI)+len(byURI))
	for uri := range oldByURI {
		affected[uri] = struct{}{}
	}
	for uri := range byURI {
		affected[uri] = struct{}{}
	}

	open := make(map[string]Document)
	for _, doc := range s.snapshotOpenDocuments() {
		open[doc.URI] = doc
	}
	for uri := range affected {
		doc, ok := open[uri]
		if !ok {
			// Findings may belong to a file in the bundle that is not open.
			// Publishing them makes them visible in the Problems panel; when
			// the file opens, DidOpen merges the cache with structural results.
			s.publishDiagnosticsAsync(uri, 0, nil)
			continue
		}
		structural := s.diagnosticsForCached(uri, doc.Text)
		s.publishDiagnosticsAsync(uri, doc.Version, structural)
	}

	errors, warnings := 0, 0
	details := make([]string, 0, len(findings))
	for _, finding := range findings {
		switch finding.Severity {
		case "error":
			errors++
		case "warning":
			warnings++
		}
		details = append(details, formatPostRenderFinding(finding))
	}
	message := ""
	if len(findings) == 1 && findings[0].Code == "post-render-error" {
		message = findings[0].Message
	}
	return postRenderResult{
		Files:    len(files),
		Errors:   errors,
		Warnings: warnings,
		Message:  message,
		Details:  details,
	}
}

// formatPostRenderFinding renders one finding as "file:line:column: severity:
// reason" so the extension can show the location and reason together instead
// of only a count. Findings without a resolved location still name the file.
// Fix guidance is appended with the field that should change.
func formatPostRenderFinding(f postrendervalidator.Finding) string {
	location := formatFindingLocation(f.Path, f.Line, f.Column)
	reason := f.Message
	if reason == "" {
		reason = f.Code
	}
	finding := fmt.Sprintf("%s: %s", f.Severity, reason)
	if location != "" {
		finding = fmt.Sprintf("%s: %s: %s", location, f.Severity, reason)
	}
	if f.Fix == "" {
		return finding
	}
	fixLocation := formatFindingLocation(f.FixPath, f.FixLine, f.FixColumn)
	if fixLocation == "" {
		return fmt.Sprintf("%s | Fix: %s", finding, f.Fix)
	}
	return fmt.Sprintf("%s | Fix at %s: %s", finding, fixLocation, f.Fix)
}

// formatFindingLocation renders "file:line:column", omitting the line and
// column when they were not resolved.
func formatFindingLocation(path string, line, column int) string {
	location := path
	if location != "" {
		location = filepath.Base(location)
	}
	if line > 0 {
		location += fmt.Sprintf(":%d", line)
		if column > 0 {
			location += fmt.Sprintf(":%d", column)
		}
	}
	return location
}

func postRenderDiagnostic(finding postrendervalidator.Finding) protocol.Diagnostic {
	line := finding.Line
	column := finding.Column
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
	severity := protocol.DiagnosticSeverityWarning
	switch finding.Severity {
	case "error":
		severity = protocol.DiagnosticSeverityError
	case "info":
		severity = protocol.DiagnosticSeverityInformation
	case "hint":
		severity = protocol.DiagnosticSeverityHint
	}
	message := finding.Message
	if finding.Fix != "" {
		message += "\nFix: " + finding.Fix
	}
	return protocol.Diagnostic{
		Range:    protocol.Range{Start: start, End: end},
		Severity: &severity,
		Code:     &protocol.IntegerOrString{Value: finding.Code},
		Source:   stringPtr(postRenderSource),
		Message:  message,
	}
}

// mergePostRenderDiagnostics appends the cached post-render findings to a
// structural publish. Keeping them separate is what lets late, debounced
// structural validation complete without erasing post-render results.
func (s *Server) mergePostRenderDiagnostics(uri string, diags []protocol.Diagnostic) []protocol.Diagnostic {
	s.postRenderMu.Lock()
	cached := append([]protocol.Diagnostic(nil), s.postRenderByURI[uri]...)
	s.postRenderMu.Unlock()
	if len(cached) == 0 {
		return diags
	}
	merged := make([]protocol.Diagnostic, 0, len(diags)+len(cached))
	merged = append(merged, diags...)
	merged = append(merged, cached...)
	return merged
}

func (s *Server) documentText(uri string) (string, bool) {
	if doc := s.GetDocument(uri); doc != nil {
		return doc.Text, true
	}
	data, err := os.ReadFile(fileURIToPath(uri))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func pathToDocumentURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// invalidatePostRenderForEdit clears the previous result when any file used by
// it changes, and republishes structural diagnostics for the affected open
// documents so stale post-render findings disappear immediately.
func (s *Server) invalidatePostRenderForEdit(uri string) {
	path := filepath.Clean(fileURIToPath(uri))
	s.postRenderMu.Lock()
	if _, ok := s.postRenderFiles[path]; !ok {
		s.postRenderMu.Unlock()
		return
	}
	s.postRenderEpoch++
	uris := keysOfDiagnosticMap(s.postRenderByURI)
	s.postRenderByURI = make(map[string][]protocol.Diagnostic)
	s.postRenderFiles = make(map[string]struct{})
	s.postRenderMu.Unlock()

	s.republishStructural(uris, "")
}

// invalidatePostRenderForClose invalidates the whole result when a file that
// contributed to it closes. The closing document itself is cleared by the
// normal didClose publish.
func (s *Server) invalidatePostRenderForClose(uri string) {
	path := filepath.Clean(fileURIToPath(uri))
	s.postRenderMu.Lock()
	if _, ok := s.postRenderFiles[path]; !ok {
		s.postRenderMu.Unlock()
		return
	}
	s.postRenderEpoch++
	uris := keysOfDiagnosticMap(s.postRenderByURI)
	s.postRenderByURI = make(map[string][]protocol.Diagnostic)
	s.postRenderFiles = make(map[string]struct{})
	s.postRenderMu.Unlock()

	s.republishStructural(uris, uri)
}

// clearPostRenderAll drops post-render findings, e.g. when the template version
// changes and every previous finding is stale. It returns the URIs that had
// findings so closed files can be cleared too.
func (s *Server) clearPostRenderAll() []string {
	s.postRenderMu.Lock()
	s.postRenderEpoch++
	uris := keysOfDiagnosticMap(s.postRenderByURI)
	s.postRenderByURI = make(map[string][]protocol.Diagnostic)
	s.postRenderFiles = make(map[string]struct{})
	s.postRenderMu.Unlock()
	return uris
}

func (s *Server) republishStructural(uris []string, skipURI string) {
	open := make(map[string]Document)
	for _, doc := range s.snapshotOpenDocuments() {
		open[doc.URI] = doc
	}
	for _, uri := range uris {
		if uri == skipURI {
			continue
		}
		doc, ok := open[uri]
		if !ok {
			// The file no longer holds post-render findings. It may be closed;
			// clear its published diagnostics without structural results.
			s.publishDiagnosticsAsync(uri, 0, nil)
			continue
		}
		s.publishDiagnosticsAsync(uri, doc.Version, s.diagnosticsForCached(uri, doc.Text))
	}
}

func keysOfDiagnosticMap(m map[string][]protocol.Diagnostic) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
