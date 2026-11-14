// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestPreRenderSamplePublishesDynamicFindings(t *testing.T) {
	for _, open := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "open"}[open], func(t *testing.T) { testPreRenderSamplePublication(t, open) })
	}
}

func testPreRenderSamplePublication(t *testing.T, componentOpen bool) {
	source := filepath.Join("testdata", "pre-render-schema-validation")
	dir := t.TempDir()
	var definition, component string
	for _, name := range []string{"componenttype.yaml", "component.yaml", "autoscaling-trait.yaml", "storage-trait.yaml"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "componenttype.yaml" {
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			text = regexp.MustCompile(`(?m)(        replicas:\n          type: )[^\n]+`).ReplaceAllString(text, `${1}string`)
			text = regexp.MustCompile(`(?m)(          default: )"?2"?$`).ReplaceAllString(text, `${1}"2"`)
			data = []byte(text)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		if name == "component.yaml" {
			component = string(data)
		}
		if name == "componenttype.yaml" {
			definition = string(data)
		}
	}
	s, client := newDebouncedServer(t)
	// Only the definition is open. The error belongs to a closed Component file.
	uri := fileURI(filepath.Join(dir, "componenttype.yaml"))
	s.OpenDocument(uri, definition, 1)
	if componentOpen {
		s.OpenDocument(fileURI(filepath.Join(dir, "component.yaml")), component, 1)
	}
	if err := s.DidSave(nil, &protocol.DidSaveTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: uri}}); err != nil {
		t.Fatal(err)
	}
	componentURI := fileURI(filepath.Join(dir, "component.yaml"))
	deadline := time.Now().Add(5 * time.Second)
	if err := client.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	for time.Now().Before(deadline) {
		var frame rxFrame
		if err := stream.ReadObject(&frame); err != nil {
			t.Fatal(err)
		}
		if frame.Method != protocol.ServerTextDocumentPublishDiagnostics {
			continue
		}
		var params struct {
			URI         string `json:"uri"`
			Diagnostics []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.URI != componentURI {
			continue
		}
		matches := 0
		for _, d := range params.Diagnostics {
			if d.Code == "schema-parameters-mismatch" {
				matches++
			}
		}
		if matches == 1 {
			return
		}
		if matches > 1 {
			t.Fatalf("duplicate schema diagnostics: %+v", params.Diagnostics)
		}
		t.Fatalf("Component received no schema mismatch: %+v", params.Diagnostics)
	}
	t.Fatal("no Component diagnostics received")
}

func TestReleasePreRenderFindingsOnWire(t *testing.T) {
	source := filepath.Join("..", "..", "lint", "pre-render-validator", "testdata", "releases.yaml")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "releases.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, client := newDebouncedServer(t)
	uri := fileURI(path)
	s.OpenDocument(uri, string(data), 1)
	if err := s.DidSave(nil, &protocol.DidSaveTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: uri}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	if err := client.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	var frame rxFrame
	if err := stream.ReadObject(&frame); err != nil {
		t.Fatal(err)
	}
	var params struct {
		URI         string `json:"uri"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, d := range params.Diagnostics {
		counts[d.Code]++
	}
	if params.URI != uri || counts["schema-parameters-mismatch"] != 2 || counts["schema-environment-configs-mismatch"] != 1 || counts["pre-render-validation-failed"] != 1 {
		t.Fatalf("missing wire findings: %+v", params)
	}
}
