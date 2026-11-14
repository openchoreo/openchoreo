// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	post "github.com/openchoreo/openchoreo/tools/lint/post-render-validator"
	pre "github.com/openchoreo/openchoreo/tools/lint/pre-render-validator"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	util "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

func TestIntegerSafetySubtractBeforeClamping(t *testing.T) {
	got := toLSPPosition(parser.Position{Line: math.MinInt, Column: math.MinInt})
	if got.Line != 0 || got.Character != 0 {
		t.Fatalf("negative input wrapped: %+v; want zero position", got)
	}
}
func TestIntegerSafetyUnsignedProtocolLimit(t *testing.T) {
	if math.MaxInt == math.MaxInt32 {
		return
	}
	maximum := math.MaxInt32
	got := util.PositionIndex(maximum + 1)
	if got > math.MaxInt32 {
		t.Fatalf("position=%d exceeds LSP uinteger maximum %d", got, math.MaxInt32)
	}
}
func TestIntegerSafetyPostRenderEndAddition(t *testing.T) {
	got := postRenderDiagnostic(post.Finding{Line: 1, Column: math.MaxInt})
	if got.Range.End.Character < got.Range.Start.Character || got.Range.End.Character > math.MaxInt32 {
		t.Fatalf("invalid end after clamp: %+v", got.Range)
	}
}
func TestIntegerSafetyPreRenderEndAddition(t *testing.T) {
	got := schemaFindingToDiagnostic(pre.Finding{Line: 1, Column: math.MaxInt})
	if got.Range.End.Character < got.Range.Start.Character || got.Range.End.Character > math.MaxInt32 {
		t.Fatalf("invalid end after clamp: %+v", got.Range)
	}
}
func TestIntegerSafetyProgressMultiplication(t *testing.T) {
	s := NewServer()
	s.conn = new(jsonrpc2.Conn)
	s.reportIndexProgress(math.MaxInt, math.MaxInt)
	notification := <-s.outbox
	p := notification.params.(*protocol.ProgressParams)
	got := p.Value.(protocol.WorkDoneProgressReport)
	if got.Percentage == nil {
		t.Fatal("percentage missing")
	}
	if *got.Percentage != 100 {
		t.Fatalf("complete scan percentage=%d; want 100", *got.Percentage)
	}
}
func TestIntegerSafetyProgressDomain(t *testing.T) {
	s := NewServer()
	s.conn = new(jsonrpc2.Conn)
	s.reportIndexProgress(2, 1)
	notification := <-s.outbox
	p := notification.params.(*protocol.ProgressParams)
	got := p.Value.(protocol.WorkDoneProgressReport)
	if got.Percentage == nil {
		t.Fatal("percentage missing")
	}
	if *got.Percentage > 100 {
		t.Fatalf("out-of-domain percentage=%d; want <=100", *got.Percentage)
	}
}
func TestIntegerSafetySeverityDomain(t *testing.T) {
	got := toLSPDiagnostic(ruleengine.Diagnostic{Severity: 0})
	if got.Severity == nil {
		t.Fatal("severity missing")
	}
	if *got.Severity < protocol.DiagnosticSeverityError || *got.Severity > protocol.DiagnosticSeverityHint {
		t.Fatalf("invalid severity preserved: %d", *got.Severity)
	}
}
func TestIntegerSafetySignedDiagnosticVersion(t *testing.T) {
	const uri = "file:///signed-version.yaml"
	s := NewServer()
	s.conn = new(jsonrpc2.Conn)
	s.OpenDocument(uri, "", -1)
	s.publishDiagnosticsAsync(uri, -1, nil)
	notification := <-s.outbox
	data, err := json.Marshal(notification.params)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version *int64 `json:"version"`
	}
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version == nil || *got.Version != -1 {
		t.Fatalf("signed version changed: %s", data)
	}
}
func TestIntegerSafetySnapshotVersionValidation(t *testing.T) {
	if math.MaxInt == math.MaxInt32 {
		return
	}
	maximum := math.MaxInt32
	const uri = "file:///snapshot-version.yaml"
	s := NewServer()
	s.OpenDocument(uri, "old", 1)
	_, err := s.ExecuteCommand(nil, &protocol.ExecuteCommandParams{Command: postRenderCommand, Arguments: []any{uri, []Document{{URI: uri, Text: "new", Version: maximum + 1}}}})
	if err == nil || s.GetDocument(uri).Version != 1 {
		t.Fatalf("oversized version accepted: err=%v document=%+v", err, s.GetDocument(uri))
	}
}
func TestIntegerSafetyProgressWorkspaceGrowth(t *testing.T) {
	root := t.TempDir()
	later := filepath.Join(root, "z")
	if err := os.Mkdir(later, 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata: {name: dev}\n")
	if err := os.WriteFile(filepath.Join(root, "a.yaml"), content, 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.conn = new(jsonrpc2.Conn)
	err := s.indexer.IndexWorkspaceWithProgress(root, func(scanned, total int) {
		if scanned == 1 {
			if err := os.WriteFile(filepath.Join(later, "new.yaml"), content, 0600); err != nil {
				t.Fatal(err)
			}
		}
		s.reportIndexProgress(scanned, total)
	})
	if err != nil {
		t.Fatal(err)
	}
	reports := 0
	for len(s.outbox) > 0 {
		reports++
		notification := <-s.outbox
		params := notification.params.(*protocol.ProgressParams)
		report := params.Value.(protocol.WorkDoneProgressReport)
		if report.Percentage != nil && *report.Percentage > 100 {
			t.Fatalf("workspace growth published %d%% progress", *report.Percentage)
		}
		if reports == 2 && (report.Message == nil || *report.Message != "2/2 files") {
			t.Fatalf("inaccurate growing-workspace message: %+v", report.Message)
		}
	}
	if reports != 2 {
		t.Fatalf("progress reports=%d, want 2", reports)
	}
}

func TestIntegerSafetyInvalidSeverities(t *testing.T) {
	for _, input := range []ruleengine.Severity{ruleengine.Severity(math.MinInt), -1, 0, 5, ruleengine.Severity(math.MaxInt)} {
		got := toLSPDiagnostic(ruleengine.Diagnostic{Severity: input})
		if got.Severity == nil || *got.Severity != protocol.DiagnosticSeverityError {
			t.Fatalf("invalid severity %d did not fall back to Error: %+v", input, got)
		}
	}
}

func TestIntegerSafetySnapshotRejectionIsAtomic(t *testing.T) {
	for _, tc := range []struct{ name, version string }{
		{"above_maximum", "2147483648"}, {"below_minimum", "-2147483649"},
		{"positive_machine_extreme", "9223372036854775807"}, {"negative_machine_extreme", "-9223372036854775808"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, second := "file:///first-version.yaml", "file:///second-version.yaml"
			s := NewServer()
			s.OpenDocument(first, "old", 1)
			s.OpenDocument(second, "old", 1)
			_, err := s.ExecuteCommand(nil, &protocol.ExecuteCommandParams{Command: postRenderCommand, Arguments: []any{first, []any{
				map[string]any{"uri": first, "text": "new", "version": 2},
				map[string]any{"uri": second, "text": "new", "version": json.Number(tc.version)},
			}}})
			if err == nil {
				t.Fatal("invalid snapshot accepted")
			}
			for _, uri := range []string{first, second} {
				doc := s.GetDocument(uri)
				if doc.Version != 1 || doc.Text != "old" {
					t.Fatalf("invalid snapshot partially applied: %+v", doc)
				}
			}
		})
	}
}

func TestIntegerSafetyLargeProgressRatios(t *testing.T) {
	for _, tc := range []struct {
		name           string
		scanned, total int
		want           uint32
	}{
		{"almost_complete", math.MaxInt - 1, math.MaxInt, 99},
		{"one_of_many", 1, math.MaxInt, 0},
		{"negative_extreme", math.MinInt, math.MaxInt, 0},
		{"growing_total", math.MaxInt, 1, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			s.conn = new(jsonrpc2.Conn)
			s.reportIndexProgress(tc.scanned, tc.total)
			notification := <-s.outbox
			params := notification.params.(*protocol.ProgressParams)
			got := params.Value.(protocol.WorkDoneProgressReport)
			if got.Percentage == nil || *got.Percentage != tc.want {
				t.Fatalf("unexpected percentage for %s: %+v", tc.name, got)
			}
		})
	}
}
