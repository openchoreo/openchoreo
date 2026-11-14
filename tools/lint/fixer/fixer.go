// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package fixer repairs the typo-class problems the linter reports: a field
// name, kind or enum value spelled slightly wrong is replaced with the
// recognized name, and whitespace that cannot change the parsed document is
// normalized. Anything that would need a value invented for it - a missing
// required field, a wrong type - is left to the user.
package fixer

import (
	"bytes"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// DefaultMaxDistance is the largest edit distance a candidate name may have
// from the misspelled token and still be used as the replacement.
const DefaultMaxDistance = 3

// maxPasses bounds the repair loop. Repairing a kind changes the schema the
// document is checked against, which can expose further typos, so the fixer
// re-parses and looks again until it stops finding new fixes.
const maxPasses = 5

// Options controls a Fix run.
type Options struct {
	// MaxDistance is the largest edit distance accepted for a replacement.
	MaxDistance int
	// Format enables the whitespace normalization pass.
	Format bool
	// Skip reports whether the repair for a diagnostic at the given 1-indexed
	// line and code must be left alone, e.g. because the user suppressed it
	// with an # occ:ignore comment. A nil Skip suppresses nothing.
	Skip func(line int, code string) bool
}

// skip reports whether a repair is suppressed, treating a nil Skip as
// "nothing is suppressed".
func (o Options) skip(line int, code string) bool {
	return o.Skip != nil && o.Skip(line, code)
}

// withDefaults fills the zero values with the defaults.
func (o Options) withDefaults() Options {
	if o.MaxDistance <= 0 {
		o.MaxDistance = DefaultMaxDistance
	}
	return o
}

// Applied describes one repair made to a file.
type Applied struct {
	// Code is the diagnostic code the repair addresses.
	Code string
	// Message describes the change in the linter's reporting style.
	Message string
	// From and To are the token before and after the repair; empty for the
	// formatting pass, which has no single token to replace.
	From string
	To   string
	// Line and Column locate the replaced token, 1-indexed. They are 0 for
	// the formatting pass.
	Line   int
	Column int
}

// Result is the outcome of a Fix run.
type Result struct {
	// Data is the fixed content, or the original content when nothing was
	// repaired.
	Data []byte
	// Applied lists every repair, in the order it was made.
	Applied []Applied
	// Skipped lists the tokens that looked fixable but were left alone,
	// together with why, so a silent no-op is never mistaken for success.
	Skipped []string
}

// Fix repairs the typos and whitespace of one OpenChoreo YAML file. It never
// invents values: a key is only renamed to a name the schema already allows
// and that no sibling key uses, and a value is only replaced when a single
// allowed value is close to it. Ambiguous and unsupported cases are reported in
// Skipped. Content that is not an OpenChoreo document is returned unchanged.
func Fix(data []byte, opts Options) (Result, error) {
	opts = opts.withDefaults()
	res := Result{Data: data}
	if !parser.HasOpenChoreoAPIVersion(data) {
		return res, nil
	}

	current := data
	seenReason := make(map[string]bool)
	for pass := 0; pass < maxPasses; pass++ {
		docs, err := parser.ParseYAML(current)
		if err != nil {
			if pass == 0 {
				// A file that does not parse has no token positions to edit.
				return Result{Data: data}, err
			}
			// A re-parse can only fail if a repair produced invalid YAML;
			// keep the last state that did parse rather than write that.
			return res, nil
		}

		edits, reasons := collectEdits(docs, opts)
		for _, r := range reasons {
			if !seenReason[r] {
				seenReason[r] = true
				res.Skipped = append(res.Skipped, r)
			}
		}
		if len(edits) == 0 {
			break
		}
		lines := splitLines(string(current))
		applied := applyEdits(lines, edits)
		if len(applied) == 0 {
			break
		}
		for _, f := range applied {
			res.Applied = append(res.Applied, Applied{
				Code:    f.code,
				Message: f.message,
				From:    f.from,
				To:      f.to,
				Line:    f.edit.Line,
				Column:  f.edit.Col,
			})
		}
		current = []byte(strings.Join(lines, "\n"))
	}

	if opts.Format {
		if formatted := FormatYAML(current); !bytes.Equal(formatted, current) {
			current = formatted
			res.Applied = append(res.Applied, Applied{
				Code:    codeFormat,
				Message: "normalized YAML formatting",
			})
		}
	}

	res.Data = current
	return res, nil
}

// Changed reports whether the run modified the content.
func (r Result) Changed() bool {
	return len(r.Applied) > 0
}
