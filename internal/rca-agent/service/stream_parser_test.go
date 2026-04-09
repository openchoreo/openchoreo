// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChatResponseParser_CompleteJSON(t *testing.T) {
	t.Parallel()
	p := &chatResponseParser{}

	delta := p.push(`{"message": "Hello world"}`)
	assert.Equal(t, "Hello world", delta)
}

func TestChatResponseParser_IncrementalChunks(t *testing.T) {
	t.Parallel()
	p := &chatResponseParser{}

	d1 := p.push(`{"message": "Hel`)
	assert.Equal(t, "Hel", d1)

	d2 := p.push(`lo wor`)
	assert.Equal(t, "lo wor", d2)

	d3 := p.push(`ld"}`)
	assert.Equal(t, "ld", d3)
}

func TestChatResponseParser_NoDuplicateDeltas(t *testing.T) {
	t.Parallel()
	p := &chatResponseParser{}

	p.push(`{"message": "Hello`)
	d := p.push("")
	assert.Equal(t, "", d)
}

func TestChatResponseParser_NoMessageKey(t *testing.T) {
	t.Parallel()
	p := &chatResponseParser{}

	delta := p.push(`{"other": "value"}`)
	assert.Equal(t, "", delta)
}

func TestChatResponseParser_EmptyMessage(t *testing.T) {
	t.Parallel()
	p := &chatResponseParser{}

	delta := p.push(`{"message": ""}`)
	assert.Equal(t, "", delta)
}

func TestExtractPartialStringValue_Complete(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "Hello world"}`, "message")
	assert.Equal(t, "Hello world", result)
}

func TestExtractPartialStringValue_Partial(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "Hello wor`, "message")
	assert.Equal(t, "Hello wor", result)
}

func TestExtractPartialStringValue_EscapedQuote(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "say \"hello\""}`, "message")
	assert.Equal(t, `say "hello"`, result)
}

func TestExtractPartialStringValue_EscapedNewline(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "line1\nline2"}`, "message")
	assert.Equal(t, "line1\nline2", result)
}

func TestExtractPartialStringValue_EscapedTab(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "col1\tcol2"}`, "message")
	assert.Equal(t, "col1\tcol2", result)
}

func TestExtractPartialStringValue_EscapedBackslash(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "path\\to\\file"}`, "message")
	assert.Equal(t, `path\to\file`, result)
}

func TestExtractPartialStringValue_UnicodeEscape(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "price \u20AC100"}`, "message")
	assert.Equal(t, `price \u20AC100`, result)
}

func TestExtractPartialStringValue_IncompleteEscapeAtEnd(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": "trailing\`, "message")
	assert.Equal(t, "trailing", result)
}

func TestExtractPartialStringValue_KeyNotFound(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"other": "value"}`, "message")
	assert.Equal(t, "", result)
}

func TestExtractPartialStringValue_NoColon(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message"`, "message")
	assert.Equal(t, "", result)
}

func TestExtractPartialStringValue_NoValueQuote(t *testing.T) {
	t.Parallel()
	result := extractPartialStringValue(`{"message": `, "message")
	assert.Equal(t, "", result)
}
