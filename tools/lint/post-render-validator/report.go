// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"context"
	"errors"
	"os"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

// Finding is one post-render validation issue attributed to a source file.
// Line and Column are 1-indexed; zero means the finding could not be located
// more precisely than the fallback file.
type Finding struct {
	Path     string
	Severity string
	Code     string
	Message  string
	Line     int
	Column   int
	// Fix is a short instruction for correcting the finding. FixPath, FixLine,
	// and FixColumn identify the field the instruction applies to. They are
	// 1-indexed and follow the same zero-means-unlocated convention as Line
	// and Column.
	Fix       string
	FixPath   string
	FixLine   int
	FixColumn int
}

// ValidateFiles renders each Component in files with the resources it
// references and returns the post-render findings. Unreadable or unparseable
// files are skipped. Validating one Component at a time is essential: a folder
// can contain several Components with different ComponentTypes, and combining
// them into one bundle would apply the wrong validation rules.
func ValidateFiles(ctx context.Context, files []string) []Finding {
	if len(files) == 0 {
		return nil
	}
	sources := loadFiles(files)
	if len(sources) == 0 {
		return nil
	}
	return validateSources(ctx, sources)
}

// loadFiles parses every readable file, associating each document with its
// source path so findings can point back to the declaring YAML.
func loadFiles(files []string) []postRenderSource {
	var sources []postRenderSource
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := parser.ParseYAML(data)
		if err != nil {
			continue
		}
		for _, doc := range parsed {
			sources = append(sources, postRenderSource{doc: doc, file: path})
		}
	}
	return sources
}

// validateSources groups the documents into one bundle per Component, renders
// each bundle, and resolves findings against that bundle's own documents.
func validateSources(ctx context.Context, sources []postRenderSource) []Finding {
	var out []Finding
	for _, grouped := range bundlesFromSources(sources) {
		if grouped.err != nil {
			out = append(out, errorFinding(grouped.errPath, grouped.err)...)
			continue
		}
		diags, err := ValidateBundle(ctx, grouped.bundle)
		if err != nil {
			out = append(out, errorFinding(grouped.fallback, err)...)
			continue
		}
		for _, d := range diags {
			file, pos := resolvePostRenderDiagnostic(d, grouped.sources, grouped.fallback)
			out = append(out, newFinding(file, pos, d))
		}
	}
	return dedupeFindings(out)
}

// dedupeFindings removes repeated findings. A shared broken reference (for
// example, an Environment pointing at a DataPlane that is not in the scanned
// tree) is reported once per Component otherwise, which makes the output noisy.
func dedupeFindings(findings []Finding) []Finding {
	type key struct {
		path     string
		severity string
		code     string
		message  string
		line     int
		column   int
	}
	seen := make(map[key]bool, len(findings))
	out := findings[:0]
	for _, finding := range findings {
		k := key{
			path:     finding.Path,
			severity: finding.Severity,
			code:     finding.Code,
			message:  finding.Message,
			line:     finding.Line,
			column:   finding.Column,
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, finding)
	}
	return out
}

// newFinding builds a located finding and attaches fix guidance pointing at
// the same field. Rules are evaluated against rendered resources, so the
// resolved source position is also the best place to correct the input.
func newFinding(path string, pos parser.Position, d ruleengine.Diagnostic) Finding {
	return locatedFinding(path, pos.Line, pos.Column, severityName(d.Severity), d.Code, d.Message)
}

// locatedFinding builds a finding with its fix guidance pointing at the field
// that needs correcting.
func locatedFinding(path string, line, column int, severity, code, message string) Finding {
	return Finding{
		Path:      path,
		Severity:  severity,
		Code:      code,
		Message:   message,
		Line:      line,
		Column:    column,
		Fix:       fixForDiagnostic(code, message),
		FixPath:   path,
		FixLine:   line,
		FixColumn: column,
	}
}

// errorFinding builds a post-render preflight/render error. Selection errors
// may carry a more precise source location than the fallback path.
func errorFinding(path string, err error) []Finding {
	line, column := 0, 0
	var located *locatedError
	if errors.As(err, &located) {
		if located.path != "" {
			path = located.path
		}
		line, column = located.line, located.column
	}
	return []Finding{locatedFinding(path, line, column, "error", "post-render-error", err.Error())}
}

// locatedError attaches the source field that caused a preflight/render error
// so the reported finding can point at the configuration to fix.
type locatedError struct {
	path   string
	line   int
	column int
	err    error
}

func (e *locatedError) Error() string { return e.err.Error() }

func (e *locatedError) Unwrap() error { return e.err }

// locateError wraps err with the position of key in doc. A missing key keeps
// the document path without a precise line.
func locateError(doc *parser.DocumentNode, path, key string, err error) error {
	line, column := 0, 0
	if doc != nil && key != "" {
		if n := findKeyNode(doc.Root, key); n != nil {
			line, column = n.Line, n.Column
		}
	}
	return &locatedError{path: path, line: line, column: column, err: err}
}

// severityName maps a ruleengine severity to its string form.
func severityName(s ruleengine.Severity) string {
	switch s {
	case ruleengine.SeverityError:
		return "error"
	case ruleengine.SeverityWarning:
		return "warning"
	case ruleengine.SeverityInfo:
		return "info"
	case ruleengine.SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}
