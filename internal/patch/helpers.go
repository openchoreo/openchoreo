// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import "strings"

// splitPointer parses a JSON Pointer string into segments, unescaping each one.
// This is used when executing RFC 6902 operations on already-expanded JSON Pointers.
func splitPointer(pointer string) []string {
	return splitAndUnescapePath(pointer)
}

// splitAndUnescapePath splits a path by "/" and unescapes RFC 6901 sequences in each segment.
//
// This is the common logic used by both splitRawPath (user input paths with filters/syntax)
// and splitPointer (standard RFC 6901 JSON Pointers).
//
// RFC 6901 escaping rules:
//   - "~0" represents "~"
//   - "~1" represents "/"
//
// IMPORTANT: When using filter expressions in paths, any "/" or "~" characters in filter
// values MUST be escaped. For example:
//
//	/containers[?(@.url=='http:~1~1example.com')]/env
//	                            ↑↑   each / escaped as ~1
//
// The append marker "-" doesn't contain escape sequences (it's a special RFC 6902 token),
// but unescaping it is safe and returns "-" unchanged.
func splitAndUnescapePath(path string) []string {
	if path == "" {
		return []string{}
	}
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return []string{""}
	}
	segments := strings.Split(trimmed, "/")
	for i, seg := range segments {
		segments[i] = unescapePointerSegment(seg)
	}
	return segments
}

// escapePointerSegment encodes a segment according to RFC 6901.
//
// Order matters! We must escape "~" first, then "/", to avoid double-escaping.
// If we escaped "/" first, we'd turn "/" into "~1", then escape the "~" into "~01",
// which would decode incorrectly.
//
// Example: "app/v1" → "app~1v1"
func escapePointerSegment(seg string) string {
	seg = strings.ReplaceAll(seg, "~", "~0")
	seg = strings.ReplaceAll(seg, "/", "~1")
	return seg
}

// unescapePointerSegment decodes a segment according to RFC 6901.
//
// Order matters! We must unescape "/" first (by replacing "~1"), then "~" (by replacing "~0").
// This correctly reverses the encoding done by escapePointerSegment.
//
// Example: "app~1v1" → "app/v1"
func unescapePointerSegment(seg string) string {
	seg = strings.ReplaceAll(seg, "~1", "/")
	seg = strings.ReplaceAll(seg, "~0", "~")
	return seg
}
