// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// normalizeTraitName rewrites the first "name:" line (the resource's
// metadata.name) to want, so tests are independent of manual edits in the
// shared sample repo.
func normalizeTraitName(text, want string) string {
	re := regexp.MustCompile(`(?m)^([ \t]*name:[ \t]*)\S+$`)
	idx := re.FindStringSubmatchIndex(text)
	if idx == nil {
		return text
	}
	// idx layout: [fullStart, fullEnd, group1Start, group1End].
	// Keep the "name: " prefix (through group1 end), insert want,
	// then continue after the full match (dropping the old value).
	return text[:idx[3]] + want + text[idx[1]:]
}

// fileToURITest mirrors the server's canonical file URI form so open-document
// URIs use the same keys as the watcher and indexer.
func fileToURITest(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if !strings.HasPrefix(abs, "file://") {
		return "file://" + abs
	}
	return abs
}

// TestLiveReferenceWarningAfterResourceNameEdit drives the server exactly like
// the user's scenario: a real trait and a component-type that references it
// are opened, then metadata.name in the trait is edited (didChange). The
// server's validation cache must revalidate the component without any client
// action, surfacing an "unknown-resource-file" warning for the old name.
//
// The two manifests are vendored under testdata/ so the test runs from a clean
// checkout; upstream they come from the sample-gitops repository.
//
// The test asserts the server output directly via DiagnosticsForText rather
// than piping raw LSP frames, eliminating net.Pipe/BuffedStream reader-race
// issues that would otherwise flake under CI load.
func TestLiveReferenceWarningAfterResourceNameEdit(t *testing.T) {
	sampleRoot, err := filepath.Abs("testdata/sample-gitops")
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}
	traitPath := filepath.Join(sampleRoot, "traits", "observability-alert-rule.yaml")
	webappPath := filepath.Join(sampleRoot, "component-types", "webapp.yaml")
	if _, err := os.Stat(traitPath); err != nil {
		t.Skipf("sample-gitops not present in this checkout: %v", err)
	}

	traitBytes, err := os.ReadFile(traitPath)
	if err != nil {
		t.Fatalf("read trait: %v", err)
	}
	webappBytes, err := os.ReadFile(webappPath)
	if err != nil {
		t.Fatalf("read webapp: %v", err)
	}

	// Normalize the trait's name to what webapp references, regardless of
	// manual edits in the shared sample repo.
	traitDoc := normalizeTraitName(string(traitBytes), "observability-alert-rule")

	// Use a dedicated temp workspace to avoid watcher noise from the sample repo.
	watchRoot := t.TempDir()
	dstTrait := filepath.Join(watchRoot, "traits.yaml")
	dstWebapp := filepath.Join(watchRoot, "webapp.yaml")
	if err := os.WriteFile(dstTrait, []byte(traitDoc), 0o600); err != nil {
		t.Fatalf("write dstTrait: %v", err)
	}
	if err := os.WriteFile(dstWebapp, webappBytes, 0o600); err != nil {
		t.Fatalf("write dstWebapp: %v", err)
	}

	s, client := watchServer(t, watchRoot)
	// The server publishes diagnostics asynchronously over an unbuffered
	// net.Pipe; drain it in the background so writers never block and the
	// connection closes cleanly. This test asserts server output via
	// DiagnosticsForText, so the frames themselves are not inspected.
	watchPump(t, client)
	t.Logf("DBG after-scan ResourceNames(%d)=%v", len(s.ResourceNames()), s.ResourceNames())

	traitURI := fileToURITest(dstTrait)
	webappURI := fileToURITest(dstWebapp)

	traitDiags := s.DiagnosticsForText(traitURI, traitDoc)
	t.Logf("DBG trait diagnostics count=%d msg=%q", len(traitDiags), firstMsg(traitDiags))
	err = s.IndexFile(traitURI, []byte(traitDoc))
	t.Logf("DBG IndexFile(trait) err=%v ResourceNames(%d)=%v", err, len(s.ResourceNames()), s.ResourceNames())

	openDoc(t, s, traitURI, traitDoc, 1)
	openDoc(t, s, webappURI, string(webappBytes), 1)

	// Baseline: the reference resolves, no warning.
	t.Logf("DBG post-open: CacheSize=%d webappParsed=%v traitParsed=%v",
		s.CacheSize(), s.CacheHasParsedDocs(webappURI), s.CacheHasParsedDocs(traitURI))
	for _, d := range s.DiagnosticsForText(webappURI, string(webappBytes)) {
		t.Logf("DBG baseline webapp diag code=%v msg=%q", d.Code, d.Message)
	}
	t.Logf("DBG post-baseline: CacheSize=%d webappParsed=%v",
		s.CacheSize(), s.CacheHasParsedDocs(webappURI))
	t.Logf("DBG baseline ResourceNames(%d)=%v", len(s.ResourceNames()), s.ResourceNames())
	if hasRefDiag(t, s, webappURI, string(webappBytes)) {
		t.Fatalf("unexpected reference warning before the edit")
	}

	// Edit metadata.name in the open trait (didChange).
	renamed := strings.Replace(traitDoc, "name: observability-alert-rule", "name: observability-alert-rules", 1)
	changeDoc(t, s, traitURI, 2, protocol.TextDocumentContentChangeEventWhole{Text: renamed})
	t.Logf("DBG post-edit ResourceNames(%d) contains rule=%v rules=%v",
		len(s.ResourceNames()), containsName(s.ResourceNames(), "observability-alert-rule"),
		containsName(s.ResourceNames(), "observability-alert-rules"))

	// Wait for the debounced revalidation to settle, then read the cache directly.
	time.Sleep(400 * time.Millisecond)
	diags := s.DiagnosticsForText(webappURI, string(webappBytes))
	for _, d := range diags {
		t.Logf("DBG webapp post-edit diag: code=%v msg=%q", d.Code, d.Message)
	}
	if !hasRefDiag(t, s, webappURI, string(webappBytes)) {
		t.Fatalf("webapp.yaml never got the reference warning after the trait name edit")
	}
}

func hasRefDiag(t *testing.T, s *lsp.Server, uri, text string) bool {
	t.Helper()
	for _, d := range s.DiagnosticsForText(uri, text) {
		if d.Code != nil && d.Code.Value == unknownResourceFileCode {
			return true
		}
	}
	return false
}

func firstMsg(diags []protocol.Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}
	return diags[0].Message
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// watchPump consumes every frame the server sends over the client connection
// until it closes, preventing SendAsync writes from blocking and deadlocking
// the server's async goroutines.
func watchPump(t *testing.T, client net.Conn) {
	t.Helper()
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	go func() {
		for {
			var frame json.RawMessage
			if err := stream.ReadObject(&frame); err != nil {
				return
			}
		}
	}()
}
