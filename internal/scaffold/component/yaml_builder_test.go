// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"
)

func TestYAMLBuilder_FormatScalarValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"int64", int64(99), "99"},
		{"whole float64", float64(5), "5"},
		{"decimal float64", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"other type", struct{}{}, "{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatScalarValue(tt.value); got != tt.want {
				t.Errorf("formatScalarValue(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestYAMLBuilder_QuoteIfNeeded(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"normal", "hello", "hello"},
		{"starts with [", "[array]", "'[array]'"},
		{"starts with {", "{object}", "'{object}'"},
		{"starts with &", "&anchor", "'&anchor'"},
		{"starts with *", "*ref", "'*ref'"},
		{"starts with !", "!tag", "'!tag'"},
		{"starts with |", "|literal", "'|literal'"},
		{"starts with >", ">folded", "'>folded'"},
		{"starts with single quote", "'quoted'", "'''quoted'''"},
		{"starts with double quote", "\"quoted\"", "'\"quoted\"'"},
		{"starts with %", "%directive", "'%directive'"},
		{"starts with @", "@at", "'@at'"},
		{"starts with backtick", "`tick`", "'`tick`'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteIfNeeded(tt.input); got != tt.want {
				t.Errorf("quoteIfNeeded(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestYAMLBuilder_AddCommentedInlineArray_Floats(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddCommentedInlineArray("ports", []any{float64(8080), float64(9090)})
	assertYAMLEqual(t, `# ports: [8080, 9090]`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_AddCommentedInlineArray_Bools(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddCommentedInlineArray("flags", []any{true, false})
	assertYAMLEqual(t, `# flags: [true, false]`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_AddCommentedInlineArray_Empty(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddCommentedInlineArray("empty", []any{})
	assertYAMLEqual(t, `# empty: []`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_InCommentedMappingWithFunc(t *testing.T) {
	b := NewYAMLBuilder()
	b.InCommentedMappingWithFunc("config", func(b *YAMLBuilder) {
		b.AddField("key", "value")
	})
	want := dedent(`
		# config:
		  # key: value
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_AddSequenceScalar(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("items")
	b.AddSequenceScalar(seq, "first")
	b.AddSequenceScalar(seq, "second")
	want := dedent(`
		items:
		  - first
		  - second
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_AddInlineSequence(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddInlineSequence("tags", []string{"web", "api"})
	assertYAMLEqual(t, `tags: [web, api]`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_AddCommentedInlineSequence(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddCommentedInlineSequence("tags", []string{"web", "api"})
	assertYAMLEqual(t, `# tags: [web, api]`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_WithFootComment(t *testing.T) {
	// WithFootComment sets FootComment on the value node, but the custom encoder
	// does not render FootComments. This documents that the option is wired but
	// has no output effect today.
	b := NewYAMLBuilder()
	b.AddField("key", "value", WithFootComment("footer"))
	assertYAMLEqual(t, `key: value`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_MappingWithLineComment_NonEmpty(t *testing.T) {
	b := NewYAMLBuilder()
	b.InMapping("config", func(b *YAMLBuilder) {
		b.AddField("key", "value")
	}, WithLineComment("A comment"))
	// LineComment on a non-empty mapping is rendered on the key line.
	want := dedent(`
		config: # A comment
		  key: value
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_MappingWithLineComment_Empty(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddMapping("config", WithLineComment("A comment"))
	assertYAMLEqual(t, `config: {} # A comment`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_MappingItems(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("servers")
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {
		b.AddField("name", "server1")
		b.AddField("port", "8080")
	})
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {
		b.AddField("name", "server2")
	})
	// First field of each item renders on the dash line; remaining fields
	// are indented further.
	want := dedent(`
		servers:
		  - name: server1
		    port: 8080
		  - name: server2
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_EmptyMappingItem(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("items")
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {})
	want := dedent(`
		items:
		  - {}
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_ScalarItems(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("tags")
	b.AddSequenceScalar(seq, "web")
	b.AddSequenceScalar(seq, "api")
	want := dedent(`
		tags:
		  - web
		  - api
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_NestedMappingValue(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("entries")
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {
		b.InMapping("nested", func(b *YAMLBuilder) {
			b.AddField("inner", "value")
		})
	})
	want := dedent(`
		entries:
		  - nested:
		      inner: value
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_EmptyMappingValue(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("entries")
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {
		b.AddMapping("config")
	})
	want := dedent(`
		entries:
		  - config: {}
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_InlineArrayValue(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("entries")
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {
		b.AddInlineArray("tags", []any{"a", "b"})
	})
	want := dedent(`
		entries:
		  - tags: [a, b]
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_BlockSequence_BlockArrayValue(t *testing.T) {
	b := NewYAMLBuilder()
	seq := b.AddSequence("entries")
	b.AddSequenceMapping(seq, func(b *YAMLBuilder) {
		innerSeq := b.AddSequence("items")
		b.AddSequenceScalar(innerSeq, "x")
		b.AddSequenceScalar(innerSeq, "y")
	})
	want := dedent(`
		entries:
		  - items:
		      - x
		      - y
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_CommentedBlockSequence(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddCommentedArray("items", []any{"first", "second"})
	want := dedent(`
		# items:
		  # - first
		  # - second
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_HeadComment_EmptyLines(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddField("key", "value", WithHeadComment("line1\n\nline3"))
	// Empty lines in head comments render as blank lines, not "# ".
	want := dedent(`
		# line1

		# line3
		key: value
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestYAMLBuilder_WithLineComment_Scalar(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddField("port", "8080", WithLineComment("Container port"))
	assertYAMLEqual(t, `port: 8080 # Container port`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_WithLineComment_InlineArray(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddInlineArray("tags", []any{"web"}, WithLineComment("Tags"))
	assertYAMLEqual(t, `tags: [web] # Tags`, encodeOrFatal(t, b))
}

func TestYAMLBuilder_WithLineComment_BlockArray(t *testing.T) {
	b := NewYAMLBuilder()
	b.AddCommentedArray("items", []any{"a"}, WithLineComment("Items list"))
	want := dedent(`
		# items: # Items list
		  # - a
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}
