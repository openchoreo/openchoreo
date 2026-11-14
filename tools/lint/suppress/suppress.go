// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package suppress

import (
	"strings"
)

// Set contains suppression markers recognized inside YAML comments:
//
//	# occ:ignore <code>[, <code2> ...]   suppress the listed codes on this line
//	# occ:ignore                          suppress every code on this line
//	# occ:ignore-file <code>[...]        suppress the listed codes for the whole file
//	# occ:ignore-file                     suppress every code for the whole file
//
// A code list of "all" (or an empty list) means every code. Codes are
// separated by commas and/or whitespace.
//
// Set is built once per file from the raw source and answers per-diagnostic
// suppression queries using 1-based line numbers (the same base as the
// validation diagnostics' Range.Start.Line).
type Set struct {
	// fileCodes holds codes suppressed anywhere in the file. nil value means
	// every code is suppressed.
	fileCodes map[string]bool
	fileAll   bool
	// lineCodes holds, per line, the codes suppressed on that exact line.
	// A nil inner map value means every code on the line is suppressed.
	lineCodes map[int]map[string]bool
	lineAll   map[int]bool
}

// New scans source for suppression markers and returns the parsed set.
func New(source []byte) *Set {
	s := &Set{
		fileCodes: map[string]bool{},
		fileAll:   false,
		lineCodes: map[int]map[string]bool{},
		lineAll:   map[int]bool{},
	}
	for i, raw := range strings.Split(string(source), "\n") {
		line := i + 1
		found, codes, isFile := markerOnLine(raw)
		if !found {
			continue
		}
		if isFile {
			if codes == nil {
				s.fileAll = true
				continue
			}
			for _, c := range codes {
				s.fileCodes[c] = true
			}
			continue
		}
		if codes == nil {
			s.lineAll[line] = true
			continue
		}
		s.lineCodes[line] = map[string]bool{}
		for _, c := range codes {
			s.lineCodes[line][c] = true
		}
	}
	return s
}

// Suppressed reports whether a diagnostic at the given 1-based line and with
// the given code is suppressed. A line of 0 (unknown position) can only be
// suppressed by a file-wide rule.
func (s *Set) Suppressed(line int, code string) bool {
	if s == nil {
		return false
	}
	if s.fileAll || s.fileCodes[code] {
		return true
	}
	if s.lineAll[line] {
		return true
	}
	if m, ok := s.lineCodes[line]; ok {
		return m[code]
	}
	return false
}

// markerOnLine extracts a suppression marker from a raw source line. found
// reports whether the line carries a marker. When found is true, codes holds a
// non-nil list of specific codes, or nil to mean "all codes", and isFile
// reports whether the marker was a file-wide (occ:ignore-file) marker.
func markerOnLine(raw string) (found bool, codes []string, isFile bool) {
	// The marker must sit in a YAML comment. Find every '#' candidate and check
	// whether the text after it is a helper marker.
	// occ:ignore-file takes precedence because it shares the occ:ignore prefix.
	const ignoreFileToken = "occ:ignore-file"
	const ignoreToken = "occ:ignore"

	for {
		hash := strings.IndexByte(raw, '#')
		if hash < 0 {
			return false, nil, false
		}

		// The '#' is only a comment start when it begins the line or follows
		// whitespace, and when the text before it is not inside a double-quoted
		// string (an odd number of unescaped quotes would mean we are inside one).
		if !commentStart(raw[:hash]) {
			raw = raw[hash+1:]
			continue
		}

		rest := strings.TrimSpace(raw[hash+1:])

		if strings.HasPrefix(rest, ignoreFileToken) {
			return true, parseCodes(rest[len(ignoreFileToken):]), true
		}
		if strings.HasPrefix(rest, ignoreToken) {
			return true, parseCodes(rest[len(ignoreToken):]), false
		}

		raw = raw[hash+1:]
	}
}

// commentStart reports whether the substring before the current '#' is a valid
// comment prefix: the '#' starts the line or is preceded by whitespace, and we
// are not inside a double-quoted scalar.
func commentStart(before string) bool {
	if len(before) == 0 {
		return true
	}
	if before[len(before)-1] != ' ' && before[len(before)-1] != '\t' {
		return false
	}
	quotes := 0
	for i := 0; i < len(before); i++ {
		if before[i] == '"' {
			if i == 0 || before[i-1] != '\\' {
				quotes++
			}
		}
	}
	return quotes%2 == 0
}

// parseCodes splits the text after occ:ignore/occ:ignore-file into a code
// list, normalized for matching. An empty remainder (or the keyword "all")
// returns nil, which the caller treats as "every code".
func parseCodes(rest string) []string {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	fields := strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == ';'
	})
	var codes []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "all" {
			return nil
		}
		if f != "" {
			codes = append(codes, f)
		}
	}
	if len(codes) == 0 {
		return nil
	}
	return codes
}
