// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package specdiff

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func opts(color bool) Options {
	return Options{Kind: "componentrelease", From: "rel-1", To: "rel-2", Root: "spec", Color: color}
}

func TestRun_Changes(t *testing.T) {
	from := map[string]any{"image": "v1", "env": []any{"A"}, "old": true}
	to := map[string]any{"image": "v2", "env": []any{"A", "B"}}

	var buf bytes.Buffer
	require.NoError(t, Run(&buf, opts(false), from, to))

	assert.Equal(t, `Comparing componentrelease spec: rel-1 -> rel-2

+ spec.env[1]: "B"
~ spec.image: "v1" -> "v2"
- spec.old: true

3 changes: 1 added, 1 removed, 1 modified
`, buf.String())
}

func TestRun_SingleChangeSummary(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Run(&buf, opts(false), map[string]any{"a": 1}, map[string]any{"a": 2}))
	assert.Contains(t, buf.String(), "1 change: 0 added, 0 removed, 1 modified")
}

func TestRun_Identical(t *testing.T) {
	var buf bytes.Buffer
	v := map[string]any{"image": "v1"}
	require.NoError(t, Run(&buf, opts(false), v, v))
	assert.Equal(t, "No differences in spec between componentrelease \"rel-1\" and \"rel-2\"\n", buf.String())
}

func TestRun_CompareError(t *testing.T) {
	var buf bytes.Buffer
	assert.Error(t, Run(&buf, opts(false), func() {}, nil))
	assert.Empty(t, buf.String())
}

func TestRender_Color(t *testing.T) {
	changes := []Change{
		{Path: "spec.a", Type: Added, New: "x"},
		{Path: "spec.b", Type: Removed, Old: "y"},
		{Path: "spec.c", Type: Modified, Old: "1", New: "2"},
	}

	var colored bytes.Buffer
	Render(&colored, opts(true), changes)
	out := colored.String()
	assert.Contains(t, out, ansiGreen+`+ spec.a: "x"`+ansiReset)
	assert.Contains(t, out, ansiRed+`- spec.b: "y"`+ansiReset)
	assert.Contains(t, out, ansiYellow+`~ spec.c: "1" -> "2"`+ansiReset)

	var plain bytes.Buffer
	Render(&plain, opts(false), changes)
	assert.NotContains(t, plain.String(), "\033[")
}

func TestParseColorMode(t *testing.T) {
	for in, want := range map[string]ColorMode{"": ColorAuto, "auto": ColorAuto, "always": ColorAlways, "never": ColorNever} {
		got, err := ParseColorMode(in)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := ParseColorMode("sometimes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid --color value "sometimes"`)
}

func TestColorMode_Enabled(t *testing.T) {
	// A regular file is never a terminal, so auto mode leaves color off.
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	require.NoError(t, err)
	defer f.Close()

	assert.True(t, ColorAlways.Enabled(f))
	assert.False(t, ColorNever.Enabled(f))
	assert.False(t, ColorAuto.Enabled(f))
}

func TestColorFlag(t *testing.T) {
	cmd := &cobra.Command{}
	AddColorFlag(cmd)

	mode, err := GetColorMode(cmd)
	require.NoError(t, err)
	assert.Equal(t, ColorAuto, mode)

	require.NoError(t, cmd.Flags().Set("color", "never"))
	mode, err = GetColorMode(cmd)
	require.NoError(t, err)
	assert.Equal(t, ColorNever, mode)

	require.NoError(t, cmd.Flags().Set("color", "bogus"))
	_, err = GetColorMode(cmd)
	assert.Error(t, err)
}
