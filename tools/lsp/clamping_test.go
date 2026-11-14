// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	postrendervalidator "github.com/openchoreo/openchoreo/tools/lint/post-render-validator"
	prerendervalidator "github.com/openchoreo/openchoreo/tools/lint/pre-render-validator"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

func TestDiagnosticPositionConversions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input parser.Position
		want  protocol.Position
	}{
		{"one_based_origin", parser.Position{Line: 1, Column: 1}, protocol.Position{}},
		{"missing_location", parser.Position{}, protocol.Position{}},
		{"negative_location", parser.Position{Line: -42, Column: -7}, protocol.Position{}},
		{"normal_location", parser.Position{Line: 17, Column: 25}, protocol.Position{Line: 16, Character: 24}},
		{"signed_upper_boundary", parser.Position{Line: math.MaxInt32, Column: math.MaxInt32}, protocol.Position{Line: math.MaxInt32 - 1, Character: math.MaxInt32 - 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := toLSPPosition(tc.input); got != tc.want {
				t.Fatalf("position = %+v, want %+v", got, tc.want)
			}
			r := toLSPRange(parser.Range{Start: tc.input, End: tc.input})
			if r.Start != tc.want || r.End != tc.want {
				t.Fatalf("range = %+v, want both endpoints %+v", r, tc.want)
			}
		})
	}
}

func TestRenderFindingPositionConversions(t *testing.T) {
	const message = "clamping regression diagnostic"
	converters := []struct {
		name    string
		convert func(int, int) protocol.Diagnostic
	}{
		{"pre_render", func(line, column int) protocol.Diagnostic {
			return schemaFindingToDiagnostic(prerendervalidator.Finding{Line: line, Column: column, Message: message, Severity: prerendervalidator.SeverityError})
		}},
		{"post_render", func(line, column int) protocol.Diagnostic {
			return postRenderDiagnostic(postrendervalidator.Finding{Line: line, Column: column, Message: message, Severity: valueError})
		}},
	}
	for _, converter := range converters {
		t.Run(converter.name, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				line, column int
				want         protocol.Position
			}{
				{"origin", 1, 1, protocol.Position{}},
				{"missing", 0, 0, protocol.Position{}},
				{"negative", -2, -3, protocol.Position{}},
				{"machine_minimum", math.MinInt, math.MinInt, protocol.Position{}},
				{"normal", 17, 25, protocol.Position{Line: 16, Character: 24}},
				{"largest_safe_character", math.MaxInt32, math.MaxInt32, protocol.Position{Line: math.MaxInt32 - 1, Character: math.MaxInt32 - 1}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got := converter.convert(tc.line, tc.column)
					if got.Range.Start != tc.want || got.Range.End.Line != tc.want.Line || got.Range.End.Character != tc.want.Character+1 {
						t.Fatalf("unexpected range: %+v", got.Range)
					}
					if got.Message != message || got.Severity == nil || *got.Severity != protocol.DiagnosticSeverityError {
						t.Fatalf("diagnostic changed: %+v", got)
					}
				})
			}
		})
	}
}

func TestDiagnosticSeverityMapping(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input ruleengine.Severity
		want  protocol.DiagnosticSeverity
	}{
		{"error", ruleengine.SeverityError, protocol.DiagnosticSeverityError},
		{"warning", ruleengine.SeverityWarning, protocol.DiagnosticSeverityWarning},
		{"information", ruleengine.SeverityInfo, protocol.DiagnosticSeverityInformation},
		{"hint", ruleengine.SeverityHint, protocol.DiagnosticSeverityHint},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := toLSPDiagnostic(ruleengine.Diagnostic{Severity: tc.input, Code: "integer-regression", Message: tc.name, Range: parser.Range{Start: parser.Position{Line: 1, Column: 1}, End: parser.Position{Line: 1, Column: 2}}})
			if got.Severity == nil || *got.Severity != tc.want || got.Message != tc.name || got.Code == nil || got.Code.Value != "integer-regression" {
				t.Fatalf("unexpected diagnostic: %+v", got)
			}
		})
	}
}

func TestIndexProgressPercentageBounds(t *testing.T) {
	for _, tc := range []struct {
		name           string
		scanned, total int
		want           uint32
		publish        bool
	}{
		{"no_total", 1, 0, 0, false}, {"negative_total", 1, -1, 0, false},
		{"not_started", 0, 10, 0, true}, {"negative_scanned", -1, 10, 0, true},
		{"fraction_truncates", 1, 3, 33, true}, {"half", 1, 2, 50, true}, {"complete", 10, 10, 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			// Retain notifications without starting a writer or a network connection.
			s.conn = new(jsonrpc2.Conn)
			s.reportIndexProgress(tc.scanned, tc.total)
			if !tc.publish {
				if len(s.outbox) != 0 {
					t.Fatal("invalid total published progress")
				}
				return
			}
			select {
			case notification := <-s.outbox:
				params, ok := notification.params.(*protocol.ProgressParams)
				if !ok {
					t.Fatalf("unexpected notification: %T", notification.params)
				}
				report, ok := params.Value.(protocol.WorkDoneProgressReport)
				if !ok || report.Percentage == nil || *report.Percentage != tc.want {
					t.Fatalf("unexpected progress: %+v", params.Value)
				}
			default:
				t.Fatal("progress notification missing")
			}
		})
	}
}

func TestPublishedDiagnosticVersionConversions(t *testing.T) {
	const uri = "file:///integer-versions.yaml"
	for _, tc := range []struct {
		name    string
		version int
	}{
		{"zero", 0}, {"normal", 1},
		{"minimum_signed", math.MinInt32}, {"maximum", math.MaxInt32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			s.conn = new(jsonrpc2.Conn)
			s.OpenDocument(uri, "", tc.version)
			s.publishDiagnosticsAsync(uri, tc.version, nil)
			select {
			case notification := <-s.outbox:
				data, err := json.Marshal(notification.params)
				if err != nil {
					t.Fatal(err)
				}
				var params struct {
					Version *int64 `json:"version"`
					URI     string `json:"uri"`
				}
				if err := json.Unmarshal(data, &params); err != nil {
					t.Fatal(err)
				}
				if params.Version == nil || *params.Version != int64(tc.version) || params.URI != uri {
					t.Fatalf("version %d changed: %s", tc.version, data)
				}
			default:
				t.Fatalf("version %d did not publish", tc.version)
			}
		})
	}
	t.Run("closed_document", func(t *testing.T) {
		s := NewServer()
		s.conn = new(jsonrpc2.Conn)
		s.publishDiagnosticsAsync(uri, 0, nil)
		select {
		case notification := <-s.outbox:
			data, err := json.Marshal(notification.params)
			if err != nil {
				t.Fatal(err)
			}
			var params struct {
				Version *int64 `json:"version"`
			}
			if err := json.Unmarshal(data, &params); err != nil {
				t.Fatal(err)
			}
			if params.Version != nil {
				t.Fatalf("closed document must omit version: %s", data)
			}
		default:
			t.Fatal("closed document did not publish")
		}
	})
	t.Run("stale_document", func(t *testing.T) {
		s := NewServer()
		s.conn = new(jsonrpc2.Conn)
		s.OpenDocument(uri, "", 2)
		s.publishDiagnosticsAsync(uri, 1, nil)
		if len(s.outbox) != 0 {
			t.Fatal("stale version published diagnostics")
		}
	})
}

func TestSnapshotDocumentVersionConversions(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		current, incoming, want int
	}{
		{"zero", -1, 0, 0}, {"increment", 0, 1, 1}, {"maximum", 1, math.MaxInt32, math.MaxInt32}, {"older_snapshot", 2, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uri := pathToDocumentURI(filepath.Join(t.TempDir(), "component.yaml"))
			s := NewServer()
			s.OpenDocument(uri, "old", tc.current)
			_, err := s.ExecuteCommand(nil, &protocol.ExecuteCommandParams{Command: postRenderCommand, Arguments: []any{uri, []Document{{URI: uri, Text: "new", Version: tc.incoming}}}})
			if err != nil {
				t.Fatal(err)
			}
			got := s.GetDocument(uri)
			if got == nil || got.Version != tc.want {
				t.Fatalf("version conversion: got %+v, want %d", got, tc.want)
			}
			if tc.incoming < tc.current && got.Text != "old" {
				t.Fatal("older snapshot replaced current text")
			}
		})
	}
}
