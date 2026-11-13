// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logformat

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	for _, format := range []string{
		"", "text", "json", "custom-columns=log", "custom-columns=timestamp,level,metadata.podName,log",
		`custom-columns=labels.openchoreo\.dev/plane,log`, `custom-columns=labels.path\\key`,
	} {
		assert.NoError(t, Validate(format), format)
	}
	for _, format := range []string{
		"yaml", "custom-columns", "custom-columns=", "custom-columns=,log", "custom-columns=timestamp,",
		"custom-columns=timestamp,,log", "custom-columns=metadata..podName", "custom-columns=.log",
		"custom-columns=metadata.", "custom-columns=metadata. podName", "custom-columns=log message",
		`custom-columns=labels.openchoreo\`, `custom-columns=labels.openchoreo\q`,
	} {
		assert.Error(t, Validate(format), format)
	}
}

func TestParseColumns_RejectNamedColumns(t *testing.T) {
	for _, tc := range []struct{ fields, invalidField string }{
		{"TIME:.timestamp,MSG:.log", "TIME:.timestamp"},
		{"TIME:timestamp,MSG:log", "TIME:timestamp"},
		{"TIME:.timestamp,log", "TIME:.timestamp"},
		{"timestamp,MSG:.log", "MSG:.log"},
		{"timestamp, MSG:.log ", "MSG:.log"},
		{":.log", ":.log"},
		{"labels.example:key", "labels.example:key"},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			format := "custom-columns=" + tc.fields
			want := `invalid custom column "` + tc.invalidField + `": named columns (NAME:path) are not supported; use custom-columns=timestamp,log`
			columns, err := ParseColumns(format)
			assert.Nil(t, columns)
			assert.EqualError(t, err, want)
			assert.EqualError(t, Validate(format), want)
		})
	}
}

func TestColumnsPrint_EscapedPaths(t *testing.T) {
	entry := map[string]any{
		"log": "request handled",
		"labels": map[string]any{
			"openchoreo.dev/plane": "controlplane",
			"openchoreo":           map[string]any{"dev/plane": "nested-plane"},
			`path\key`:             "backslash",
		},
		"literal.key": map[string]any{"name.with.dots": "value"},
	}
	for _, tc := range []struct {
		name   string
		format string
		want   string
	}{
		{"literal label key", `custom-columns=labels.openchoreo\.dev/plane,log`, "controlplane\trequest handled\n"},
		{"unescaped periods remain nested", "custom-columns=labels.openchoreo.dev/plane,log", "nested-plane\trequest handled\n"},
		{"multiple literal keys", `custom-columns=literal\.key.name\.with\.dots`, "value\n"},
		{"literal backslash", `custom-columns=labels.path\\key`, "backslash\n"},
		{"missing literal key", `custom-columns=labels.missing\.key,log`, "-\trequest handled\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			columns, err := ParseColumns(tc.format)
			require.NoError(t, err)
			var out bytes.Buffer
			require.NoError(t, columns.Print(&out, entry))
			assert.Equal(t, tc.want, out.String())
		})
	}
}

func TestColumnsPrint(t *testing.T) {
	entry := map[string]any{
		"timestamp": "2026-11-10T08:21:04.512Z", "log": "GET /api/v1/urls/abc123 302 1.4ms", "level": "INFO",
		"metadata": map[string]any{"podName": "snip-api-service-development-6d8f7c9b5-x2k4p", "null": nil},
		"count":    json.Number("9007199254740993"), "ready": false, "empty": "",
	}
	for _, tc := range []struct {
		name   string
		format string
		want   string
	}{
		{"example", "custom-columns=timestamp,level,metadata.podName,log",
			"2026-11-10T08:21:04.512Z\tINFO\tsnip-api-service-development-6d8f7c9b5-x2k4p\tGET /api/v1/urls/abc123 302 1.4ms\n"},
		{"order and repeated fields", "custom-columns=level,log,level", "INFO\tGET /api/v1/urls/abc123 302 1.4ms\tINFO\n"},
		{"missing and null fields", "custom-columns=missing,metadata.missing,metadata.null,level.foo,log", "-\t-\t-\t-\tGET /api/v1/urls/abc123 302 1.4ms\n"},
		{"scalar values", "custom-columns=count,ready,empty,level", "9007199254740993\tfalse\t\tINFO\n"},
		{"trim field whitespace", "custom-columns= level , metadata.podName ", "INFO\tsnip-api-service-development-6d8f7c9b5-x2k4p\n"},
		{"object value", "custom-columns=metadata", "{\"null\":null,\"podName\":\"snip-api-service-development-6d8f7c9b5-x2k4p\"}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			columns, err := ParseColumns(tc.format)
			require.NoError(t, err)
			require.NotNil(t, columns)
			var out bytes.Buffer
			require.NoError(t, columns.Print(&out, entry))
			assert.Equal(t, tc.want, out.String())
		})
	}
}
