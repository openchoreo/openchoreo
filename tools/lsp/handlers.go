// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"net/url"
	"os"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"github.com/openchoreo/openchoreo/tools/lsp/hover"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// buildVersion is stamped at release time via
// -ldflags "-X github.com/openchoreo/openchoreo/tools/lsp.buildVersion=$VERSION"
// and logged at initialize so the running binary can be identified in the
// client's server output channel.
var buildVersion = "dev"

// extractTemplateVersion pulls a template version string out of the LSP
// client's initializationOptions or configuration settings. Clients may send
// the value directly ({"templateVersion": "v1.2.0"}), nested under an
// "openchoreo" section (as workspace/didChangeConfiguration receives a
// configurationSection), as a settings map, or as a plain string. Empty when
// the client sent nothing relevant.
func extractTemplateVersion(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t["templateVersion"].(string); ok && s != "" {
			return s
		}
		// workspace/didChangeConfiguration with configurationSection wraps the
		// section content under the section name.
		if open, ok := t["openchoreo"].(map[string]any); ok {
			if s, ok := open["templateVersion"].(string); ok {
				return s
			}
		}
	case map[string]string:
		return t["templateVersion"]
	case []any:
		// Some clients deliver settings as an array of section maps.
		for _, item := range t {
			if s := extractTemplateVersion(item); s != "" {
				return s
			}
		}
	}
	return ""
}

// applyTemplateVersion selects the given template version ("" and "latest"
// resolve to the latest). Unknown versions fall back to latest so a malformed
// client value can never break validation. It returns the now-active version.
func applyTemplateVersion(version string) string {
	if version == "" || version == "latest" {
		version = template.LatestVersion
	}
	if err := template.SetVersion(version); err != nil {
		_ = template.SetVersion(template.LatestVersion)
	}
	return template.CurrentVersion()
}

// DidChangeConfiguration applies a client template-version change and
// re-publishes diagnostics for every open document under the new version.
func (s *Server) DidChangeConfiguration(
	context *glsp.Context,
	params *protocol.DidChangeConfigurationParams,
) error {
	version := extractTemplateVersion(params.Settings)
	if version == "" {
		return nil
	}
	applied := applyTemplateVersion(version)
	if s.logger != nil {
		s.logger.Info("template version changed", "version", applied)
	}
	s.invalidateValidationCache()
	stalePostRender := s.clearPostRenderAll()
	openDocs := s.snapshotOpenDocuments()
	open := make(map[string]struct{}, len(openDocs))
	for _, doc := range openDocs {
		open[doc.URI] = struct{}{}
		s.publishDiagnosticsAsync(doc.URI, doc.Version, s.diagnosticsForCached(doc.URI, doc.Text))
	}
	for _, uri := range stalePostRender {
		if _, ok := open[uri]; !ok {
			s.publishDiagnosticsAsync(uri, 0, nil)
		}
	}
	return nil
}

func (s *Server) Initialize(
	context *glsp.Context,
	params *protocol.InitializeParams,
) (any, error) {

	version := "0.2.0"

	openClose := true
	change := protocol.TextDocumentSyncKindIncremental
	save := false
	codeLensResolve := false

	// Index workspace in the background so initialize responds promptly, and
	// report $/progress when the client declares WorkDoneProgress support.
	if params.Capabilities.Window != nil &&
		params.Capabilities.Window.WorkDoneProgress != nil &&
		*params.Capabilities.Window.WorkDoneProgress {
		s.indexProgress = true
	}

	roots := resolveWorkspaceRoots(params)
	s.workspaceRootsMu.Lock()
	s.workspaceRoots = roots
	s.workspaceRootsMu.Unlock()
	if len(roots) > 0 {
		s.initialIndexStarted = true
		s.beginIndexing(roots)
	}

	if s.logger != nil {
		s.logger.Info("OpenChoreo LSP initialized",
			"build", buildVersion,
			"roots", roots)
	}

	// The client may select a template version via initializationOptions
	// (industry-standard LSP config channel). Defaults to the latest version.
	var activeTemplate string
	if opts := params.InitializationOptions; opts != nil {
		if v := extractTemplateVersion(opts); v != "" {
			activeTemplate = applyTemplateVersion(v)
		}
	}
	if activeTemplate == "" {
		activeTemplate = applyTemplateVersion(template.LatestVersion)
	}
	if s.logger != nil {
		s.logger.Info("template version selected", "version", activeTemplate)
	}

	// Surface schema/hover config load failures instead of crashing.
	template.AllKinds()
	if err := template.LoadError(); err != nil {
		s.notifyShow(context, protocol.MessageTypeWarning,
			"Schema registry failed to load: "+err.Error())
		s.notifyLog(context, protocol.MessageTypeError,
			"template.LoadError: "+err.Error())
	}
	if err := hover.LoadError(); err != nil {
		s.notifyShow(context, protocol.MessageTypeWarning,
			"Hover configuration failed to load: "+err.Error())
		s.notifyLog(context, protocol.MessageTypeError,
			"hover.LoadError: "+err.Error())
	}

	return protocol.InitializeResult{

		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    "OpenChoreo Language Server",
			Version: &version,
		},

		Capabilities: protocol.ServerCapabilities{
			Workspace: &protocol.ServerCapabilitiesWorkspace{WorkspaceFolders: &protocol.WorkspaceFoldersServerCapabilities{Supported: &openClose, ChangeNotifications: &protocol.BoolOrString{Value: true}}},
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: &openClose,
				Change:    &change,
				Save:      save,
			},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", "-", ":"},
			},
			HoverProvider:      true,
			DefinitionProvider: true,
			CodeActionProvider: &protocol.CodeActionOptions{
				CodeActionKinds: []protocol.CodeActionKind{protocol.CodeActionKindQuickFix},
			},
			CodeLensProvider: &protocol.CodeLensOptions{
				ResolveProvider: &codeLensResolve,
			},
			ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
				Commands: []string{postRenderCommand, "openchoreo.requiredFieldsPreview"},
			},
		},
	}, nil
}

func (s *Server) DidOpen(
	context *glsp.Context,
	params *protocol.DidOpenTextDocumentParams,
) error {

	uri := string(params.TextDocument.URI)
	s.OpenDocument(
		uri,
		params.TextDocument.Text,
		int(params.TextDocument.Version),
	)

	// Update indexer with new file content
	if err := s.indexer.UpdateFile(uri, []byte(params.TextDocument.Text)); err != nil {
		s.notifyLog(context, protocol.MessageTypeError,
			"indexer update failed for "+uri+": "+err.Error())
	}

	// The opened document's resources now exist under its declared names;
	// other open documents may reference them.
	s.scheduleOtherDocRevalidation(uri)
	if path := fileURIToPath(uri); path != "" {
		go s.triggerPreRenderSchemaValidationForPath(path)
	}

	s.publishDiagnostics(
		context,
		uri,
		int(params.TextDocument.Version),
		s.diagnosticsForCached(uri, params.TextDocument.Text),
	)

	return nil
}

func (s *Server) DidChange(
	context *glsp.Context,
	params *protocol.DidChangeTextDocumentParams,
) error {

	uri := string(params.TextDocument.URI)

	doc, err := s.applyChangesToDocument(uri, params)
	if err != nil {
		return err
	}

	// Update indexer with new content
	if err := s.indexer.UpdateFile(uri, []byte(doc.Text)); err != nil {
		s.notifyLog(context, protocol.MessageTypeError,
			"indexer update failed for "+uri+": "+err.Error())
	}

	// Any edit to a file used by the last post-render run makes the whole
	// result stale. Clear it immediately rather than waiting for a rerun.
	s.invalidatePostRenderForEdit(uri)

	// The edited document's resource names may have changed; references to
	// them in other open documents need rechecking too, not just this one.
	s.scheduleOtherDocRevalidation(uri)

	s.scheduleDiagnostics(uri, doc.Version, doc.Text)

	return nil
}

func (s *Server) DidClose(
	context *glsp.Context,
	params *protocol.DidCloseTextDocumentParams,
) error {

	uri := string(params.TextDocument.URI)
	closeChanged := false
	if current := s.GetDocument(uri); current != nil {
		saved, err := os.ReadFile(fileURIToPath(uri))
		closeChanged = err != nil || string(saved) != current.Text
	}
	s.CloseDocument(uri)
	s.cancelDiagnostics(uri)
	s.deleteValidationEntry(uri)
	if closeChanged {
		s.invalidatePostRenderForClose(uri)
	}

	// A document closed after its file was renamed or deleted must not keep
	// its old name in the index. Files that still exist on disk are untouched.
	if _, err := os.Stat(fileURIToPath(uri)); err != nil {
		s.indexer.RemoveFile(uri)
	} else {
		s.updateIndexedFile(fileURIToPath(uri))
	}

	// Closing may have removed resource names other open documents reference.
	s.scheduleOtherDocRevalidation(uri)

	s.publishDiagnostics(context, uri, 0, nil)
	if path := fileURIToPath(uri); path != "" {
		go s.triggerPreRenderSchemaValidationForPath(path)
	}

	return nil
}

// fileURIToPath converts a file:// URI to a local filesystem path.
func fileURIToPath(uri string) string {
	// Handle file:// URIs
	if strings.HasPrefix(uri, "file://") {
		path := strings.TrimPrefix(uri, "file://")
		// URL decode the path (handles spaces and special characters)
		decoded, err := url.PathUnescape(path)
		if err == nil {
			return decoded
		}
		return path
	}
	return uri
}

func (s *Server) DidSave(
	context *glsp.Context,
	params *protocol.DidSaveTextDocumentParams,
) error {
	uri := string(params.TextDocument.URI)
	if path := fileURIToPath(uri); path != "" {
		go s.triggerPreRenderSchemaValidationForPath(path)
	}
	return nil
}
