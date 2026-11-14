// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	postrendervalidator "github.com/openchoreo/openchoreo/tools/lint/post-render-validator"
	prerendervalidator "github.com/openchoreo/openchoreo/tools/lint/pre-render-validator"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
	codecompletion "github.com/openchoreo/openchoreo/tools/lsp/code-completion"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
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
	Root     string   `json:"root,omitempty"`
}

// CodeLens offers the post-render action at the top of OpenChoreo YAML files.
func (s *Server) CodeLens(
	context *glsp.Context,
	params *protocol.CodeLensParams,
) ([]protocol.CodeLens, error) {
	if params == nil {
		return nil, fmt.Errorf("missing codeLens parameters")
	}
	uri := params.TextDocument.URI
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
			Title:     "Validate Project",
			Command:   postRenderCommand,
			Arguments: []any{uri},
		},
	}}, nil
}

// ExecuteCommand handles openchoreo.runPostRenderValidation. The command takes
// the document URI as its only argument and validates its whole workspace.
func (s *Server) ExecuteCommand(
	context *glsp.Context,
	params *protocol.ExecuteCommandParams,
) (any, error) {
	if params == nil {
		return nil, fmt.Errorf("missing executeCommand parameters")
	}
	if params.Command == "openchoreo.requiredFieldsPreview" {
		var request struct {
			URI       string
			Text      string
			Version   int
			Line      int
			Character int
			Indent    string
		}
		if len(params.Arguments) != 1 {
			return nil, fmt.Errorf("missing preview input")
		}
		data, err := json.Marshal(params.Arguments[0])
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return map[string]any{"version": request.Version, "text": codecompletion.RequiredPreviewWithIndex(request.Text, request.Line, request.Character, request.Indent, request.URI, s.indexer)}, nil
	}
	if params.Command != postRenderCommand {
		return nil, fmt.Errorf("unknown command %q", params.Command)
	}
	uri, err := postRenderURI(params.Arguments)
	if err != nil {
		return nil, err
	}
	// The editor supplies an authoritative snapshot with manual requests. This
	// also repairs stale server buffers before the validation epoch is captured.
	if len(params.Arguments) > 1 {
		data, e := json.Marshal(params.Arguments[1])
		if e != nil {
			return nil, e
		}
		var documents []Document
		if e = json.Unmarshal(data, &documents); e != nil {
			return nil, fmt.Errorf("invalid editor snapshot: %w", e)
		}
		// Validate the complete snapshot before mutating any editor buffers.
		for _, doc := range documents {
			if doc.Version < math.MinInt32 || doc.Version > math.MaxInt32 {
				return nil, fmt.Errorf("document version %d is outside the signed 32-bit range", doc.Version)
			}
		}
		for _, doc := range documents {
			current := s.GetDocument(doc.URI)
			if current == nil || doc.Version < current.Version || doc.Text == current.Text {
				continue
			}
			if fileURIToPath(doc.URI) == "" {
				continue
			}
			if e := s.DidChange(nil, &protocol.DidChangeTextDocumentParams{
				TextDocument:   protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: doc.URI}, Version: lsputil.Integer(doc.Version)},
				ContentChanges: []any{protocol.TextDocumentContentChangeEventWhole{Text: doc.Text}},
			}); e != nil {
				return nil, e
			}
		}
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
	s.postRenderMu.Lock()
	s.postRenderScope = root
	s.postRenderMu.Unlock()
	files, err := parser.CollectYAMLFiles(root, true)
	if err != nil {
		return s.publishPostRenderFailure(uri, activePath, err)
	}
	files = ensureActiveFile(files, activePath)

	ctx, cancel := context.WithTimeout(context.Background(), postRenderTimeout)
	defer cancel()
	overlays := map[string]string{}
	for _, doc := range s.snapshotOpenDocuments() {
		path := fileURIToPath(doc.URI)
		rel, e := filepath.Rel(root, path)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			overlays[filepath.Clean(path)] = doc.Text
			files = ensureActiveFile(files, path)
		}
	}
	schemaOverlays := map[string][]byte{}
	for path, text := range overlays {
		schemaOverlays[path] = []byte(text)
	}
	schemaScope := s.freshTargetSchemaScope(uri, schemaOverlays)
	var parsed []postrendervalidator.SourceDocument
	var pre []prerendervalidator.Resource
	var findings []postrendervalidator.Finding
	snapshotHashes := map[string][32]byte{}
	for _, path := range files {
		text, ok := overlays[filepath.Clean(path)]
		if !ok {
			data, exists := schemaScope.Content(path)
			var e error
			if !exists {
				data, e = os.ReadFile(path)
			}
			if e != nil {
				continue
			}
			text = string(data)
		}
		snapshotHashes[filepath.Clean(path)] = sha256.Sum256([]byte(text))
		if !parser.HasOpenChoreoAPIVersion([]byte(text)) {
			continue
		}
		docs, e := parser.ParseYAML([]byte(text))
		var diagnostics []protocol.Diagnostic
		if e != nil {
			diagnostics = []protocol.Diagnostic{parseErrorDiagnostic(e)}
		} else {
			for _, doc := range docs {
				doc.ValidationContext = targetschema.WithResolver(doc.ValidationContext, schemaScope)
			}
			diagnostics = validateDocs(docs)
		}
		for _, d := range diagnostics {
			code := ""
			if d.Code != nil {
				code = fmt.Sprint(d.Code.Value)
			}
			severity := valueError
			if d.Severity != nil {
				switch *d.Severity {
				case protocol.DiagnosticSeverityWarning:
					severity = "warning"
				case protocol.DiagnosticSeverityInformation:
					severity = "info"
				case protocol.DiagnosticSeverityHint:
					severity = "hint"
				}
			}
			findings = append(findings, postrendervalidator.Finding{Path: path, Code: code, Severity: severity, Message: d.Message, Line: int(d.Range.Start.Line) + 1, Column: int(d.Range.Start.Character) + 1})
		}
		for _, doc := range docs {
			parsed = append(parsed, postrendervalidator.SourceDocument{Path: path, Document: doc})
			pre = append(pre, prerendervalidator.Resource{Path: path, Document: doc})
		}
	}
	for _, f := range prerendervalidator.ValidateDocuments(ctx, pre) {
		findings = append(findings, postrendervalidator.Finding{Path: f.Path, Code: f.Code, Severity: string(f.Severity), Message: f.Message, Line: f.Line, Column: f.Column})
	}
	findings = append(findings, postrendervalidator.ValidateWorkspace(ctx, parsed)...)

	result := s.publishPostRenderFindings(uri, activePath, files, uniqueManualFindings(findings), &epoch, snapshotHashes)
	result.Root = root
	return result
}

// postRenderRoot uses the containing VS Code workspace folder. Standalone
// documents fall back to their directory; unrelated workspace folders are excluded.
func (s *Server) postRenderRoot(activePath string) string {
	activeDir := filepath.Dir(activePath)
	workspaceRoot := ""
	for _, root := range s.workspaceRootsSnapshot() {
		rel, err := filepath.Rel(root, activePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(root) > len(workspaceRoot) {
			workspaceRoot = root
		}
	}

	if workspaceRoot != "" {
		return workspaceRoot
	}
	return activeDir
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
		Severity: valueError,
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
	snapshots ...map[string][32]byte,
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
	s.postRenderContentHashes = nil
	if len(snapshots) > 0 {
		s.postRenderContentHashes = snapshots[0]
	}
	s.postRenderMu.Unlock()

	affected := make(map[string]struct{}, len(oldByURI)+len(byURI))
	for uri := range oldByURI {
		affected[uri] = struct{}{}
	}
	for uri := range byURI {
		affected[uri] = struct{}{}
	}

	for uri := range affected {
		s.republishCurrentDiagnostics(uri)
	}

	if expectedEpoch != nil {
		s.postRenderMu.Lock()
		stale := *expectedEpoch != s.postRenderEpoch
		s.postRenderMu.Unlock()
		if stale {
			return postRenderResult{Files: len(files), Message: "Files changed during post-render validation; run it again."}
		}
	}
	errors, warnings := 0, 0
	details := make([]string, 0, len(findings))
	for _, finding := range findings {
		switch finding.Severity {
		case valueError:
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
		Line:      lsputil.PositionIndex(line - 1),
		Character: lsputil.PositionIndex(column - 1),
	}
	end := protocol.Position{
		Line:      start.Line,
		Character: min(start.Character+1, math.MaxInt32),
	}
	severity := protocol.DiagnosticSeverityWarning
	switch finding.Severity {
	case valueError:
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
	current, exists := s.documentText(uri)
	currentHash := sha256.Sum256([]byte(current))
	s.postRenderMu.Lock()
	if _, ok := s.postRenderFiles[path]; !ok && !projectContains(s.postRenderScope, path) {
		s.postRenderMu.Unlock()
		return
	}
	if captured, known := s.postRenderContentHashes[path]; known && exists && captured == currentHash {
		s.postRenderMu.Unlock()
		return
	}
	s.postRenderEpoch++
	uris := keysOfDiagnosticMap(s.postRenderByURI)
	s.postRenderByURI = make(map[string][]protocol.Diagnostic)
	s.postRenderFiles = make(map[string]struct{})
	s.postRenderContentHashes = nil
	s.postRenderMu.Unlock()

	s.republishStructural(uris, "")
}

// invalidatePostRenderForClose invalidates the whole result when a file that
// contributed to it closes. The closing document itself is cleared by the
// normal didClose publish.
func (s *Server) invalidatePostRenderForClose(uri string) {
	path := filepath.Clean(fileURIToPath(uri))
	s.postRenderMu.Lock()
	if _, ok := s.postRenderFiles[path]; !ok && !projectContains(s.postRenderScope, path) {
		s.postRenderMu.Unlock()
		return
	}
	s.postRenderEpoch++
	uris := keysOfDiagnosticMap(s.postRenderByURI)
	s.postRenderByURI = make(map[string][]protocol.Diagnostic)
	s.postRenderFiles = make(map[string]struct{})
	s.postRenderContentHashes = nil
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
	s.postRenderContentHashes = nil
	s.postRenderMu.Unlock()
	return uris
}

func (s *Server) republishStructural(uris []string, skipURI string) {
	for _, uri := range uris {
		if uri != skipURI {
			s.republishCurrentDiagnostics(uri)
		}
	}
}

func keysOfDiagnosticMap(m map[string][]protocol.Diagnostic) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func projectContains(root, path string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func uniqueManualFindings(findings []postrendervalidator.Finding) []postrendervalidator.Finding {
	var out []postrendervalidator.Finding
	seen := map[string]bool{}
	for _, f := range findings {
		if f.Code == "pre-render-validation-failed" {
			duplicate := false
			for _, other := range findings {
				if other.Path == f.Path && other.Code == "post-render-error" && strings.Contains(other.Message, f.Message) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
		}
		key := fmt.Sprintf("%s:%s:%d:%d:%s", f.Path, f.Code, f.Line, f.Column, f.Message)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}
