// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"strings"
)

// chatResponseParser extracts clean message deltas from streaming structured
// JSON output. The LLM streams tokens like `{"message": "Here is the `.
// This parser extracts the "message" value incrementally without waiting
// for complete JSON, and captures "actions" when the JSON is complete.
type chatResponseParser struct {
	buffer  string
	message string
	actions []any
}

// push appends a chunk and returns the new message delta, or empty string
// if no new content could be extracted.
func (p *chatResponseParser) push(chunk string) string {
	p.buffer += chunk

	// Try complete JSON first (fast path for final chunk).
	var full map[string]any
	if json.Unmarshal([]byte(p.buffer), &full) == nil {
		if actions, ok := full["actions"]; ok {
			if arr, ok := actions.([]any); ok {
				p.actions = arr
			}
		}
		newMessage, _ := full["message"].(string)
		return p.emitDelta(newMessage)
	}

	// Partial JSON: extract "message" value by scanning the buffer.
	newMessage := extractPartialStringValue(p.buffer, "message")
	return p.emitDelta(newMessage)
}

func (p *chatResponseParser) emitDelta(newMessage string) string {
	if len(newMessage) > len(p.message) {
		delta := newMessage[len(p.message):]
		p.message = newMessage
		return delta
	}
	return ""
}

// extractPartialStringValue extracts the string value for a given key from
// a potentially incomplete JSON buffer. Handles escape sequences.
//
// For input `{"message": "Hello, wor` it returns "Hello, wor".
func extractPartialStringValue(buf, key string) string { //nolint:unparam // key is parameterized for testability
	// Find "key": "
	needle := `"` + key + `"`
	idx := strings.Index(buf, needle)
	if idx < 0 {
		return ""
	}

	// Skip past the key, colon, and opening quote.
	rest := buf[idx+len(needle):]
	// Find the colon then the opening quote of the value.
	colonIdx := strings.IndexByte(rest, ':')
	if colonIdx < 0 {
		return ""
	}
	rest = rest[colonIdx+1:]

	quoteIdx := strings.IndexByte(rest, '"')
	if quoteIdx < 0 {
		return ""
	}
	rest = rest[quoteIdx+1:]

	// Scan the string value, handling escape sequences.
	var sb strings.Builder
	i := 0
	for i < len(rest) {
		ch := rest[i]
		if ch == '"' {
			break // End of string value.
		}
		if ch == '\\' {
			if i+1 >= len(rest) {
				break // Incomplete escape at end of buffer.
			}
			next := rest[i+1]
			switch next {
			case '"', '\\', '/':
				sb.WriteByte(next)
				i += 2
			case 'n':
				sb.WriteByte('\n')
				i += 2
			case 'r':
				sb.WriteByte('\r')
				i += 2
			case 't':
				sb.WriteByte('\t')
				i += 2
			case 'u':
				// Unicode escape: \uXXXX — need 4 hex digits.
				if i+5 >= len(rest) {
					i = len(rest) // Incomplete unicode escape, stop scanning.
				} else {
					// Pass through as-is for simplicity.
					sb.WriteString(rest[i : i+6])
					i += 6
				}
			default:
				sb.WriteByte(next)
				i += 2
			}
		} else {
			sb.WriteByte(ch)
			i++
		}
	}

	return sb.String()
}
