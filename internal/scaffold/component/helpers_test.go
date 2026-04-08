// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// assertYAMLEqual performs exact string equality after stripping a trailing
// newline. Fails the test with a unified diff when the output differs.
func assertYAMLEqual(t *testing.T, want, got string) {
	t.Helper()
	got = strings.TrimRight(got, "\n")
	want = strings.TrimRight(want, "\n")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

// encodeOrFatal encodes the builder and fails the test on error.
func encodeOrFatal(t *testing.T, b *YAMLBuilder) string {
	t.Helper()
	got, err := b.Encode()
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	return got
}

// dedent strips the minimum common leading whitespace from every non-empty line.
// It also strips a single leading newline and any trailing whitespace/newlines,
// so callers can write raw-string YAML indented to match the surrounding Go code:
//
//	want := dedent(`
//	    retryPolicy: {}
//	    # retryPolicy:
//	      # attempts: 0
//	`)
//
// The example above yields "retryPolicy: {}\n# retryPolicy:\n  # attempts: 0".
// Relative indentation within the block is preserved (the "# attempts: 0" line
// stays indented two spaces past "# retryPolicy:").
//
// A leading blank line (i.e. two newlines at the start of the raw string) is
// preserved as an intentional blank output line; only one leading newline is
// consumed as structural.
func dedent(s string) string {
	s = strings.TrimPrefix(s, "\n")
	s = strings.TrimRight(s, " \t\n")

	lines := strings.Split(s, "\n")

	// Compute the minimum indent across non-blank lines.
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return s
	}

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = line[minIndent:]
	}
	return strings.Join(lines, "\n")
}
