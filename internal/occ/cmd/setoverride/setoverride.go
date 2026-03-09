// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package setoverride

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/sjson"
)

// bracketIndex matches bracket-notation array indices like [0], [1], [-1].
var bracketIndex = regexp.MustCompile(`\[(-?\d+)\]`)

// jsonNumber matches a valid JSON number per RFC 7159:
// optional minus, integer (no leading zeros except "0"), optional fraction, optional exponent.
var jsonNumber = regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$`)

// Apply parses each "key=value" entry from setValues, converts the key from
// standard JSON path notation to sjson path notation, and applies the value
// to the given JSON string. It returns the updated JSON string.
//
// Standard JSON path notation uses brackets for array indices:
//
//	spec.containers[0].name=nginx
//
// sjson expects dot-separated numeric indices:
//
//	spec.containers.0.name
//
// This function transparently converts between the two so that users can
// supply the more familiar bracket notation.
func Apply(jsonStr string, setValues []string) (string, error) {
	var err error
	for _, sv := range setValues {
		parts := strings.SplitN(sv, "=", 2)
		if len(parts) != 2 {
			return jsonStr, fmt.Errorf("invalid --set format %q, expected key=value", sv)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return jsonStr, fmt.Errorf("empty key in --set flag")
		}

		sjsonPath := ToSjsonPath(key)

		jsonStr, err = sjson.SetRaw(jsonStr, sjsonPath, toJSONLiteral(value))
		if err != nil {
			return jsonStr, fmt.Errorf("failed to set value for key %q: %w", key, err)
		}
	}
	return jsonStr, nil
}

// ToSjsonPath converts a standard JSON path (Helm-style) to sjson path syntax.
//
// Bracket notation denotes array indices, bare numeric segments are object keys:
//
//	spec.containers[0].name  → spec.containers.0.name   (array index)
//	spec.containers.0.name   → spec.containers.:0.name  (object key "0")
//	spec.ports[0]            → spec.ports.0              (array index)
//	metadata.labels          → metadata.labels           (unchanged)
func ToSjsonPath(path string) string {
	// First, replace bracket indices [N] with a placeholder that won't be
	// affected by the bare-numeric escaping pass.
	// Use \x00 as a temporary marker since it cannot appear in user input.
	result := bracketIndex.ReplaceAllString(path, ".\x00$1")

	// Escape bare numeric segments with sjson's colon prefix so they are
	// treated as object keys, not array indices.
	segments := strings.Split(result, ".")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		// Segments starting with \x00 are array indices from bracket notation.
		if seg[0] == '\x00' {
			segments[i] = seg[1:] // strip marker, keep as numeric (array index)
			continue
		}
		// Bare numeric segment → prefix with : so sjson treats it as object key.
		if isNumeric(seg) {
			segments[i] = ":" + seg
		}
	}

	result = strings.Join(segments, ".")
	// Clean up empty segments from consecutive dots.
	for strings.Contains(result, "..") {
		result = strings.ReplaceAll(result, "..", ".")
	}
	result = strings.TrimPrefix(result, ".")
	return result
}

// isNumeric returns true if s is a decimal integer (possibly negative).
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	start := 0
	if s[0] == '-' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for _, c := range s[start:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// toJSONLiteral converts a CLI string value to its raw JSON representation.
// It preserves the correct JSON type: booleans become true/false, numbers stay
// unquoted, and everything else is quoted as a JSON string.
// Numbers must conform to RFC 7159 (no leading zeros, no leading "+").
// Inputs like "01" or "+1" are treated as strings.
func toJSONLiteral(s string) string {
	if s == "true" || s == "false" || s == "null" {
		return s
	}
	if jsonNumber.MatchString(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return s
		}
	}
	b, _ := json.Marshal(s)
	return string(b)
}
