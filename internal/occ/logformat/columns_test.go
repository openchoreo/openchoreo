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
	for _, format := range []string{"", "text", "json", "custom-columns=log", "custom-columns=timestamp,level,metadata.podName,log"} {
		assert.NoError(t, Validate(format), format)
	}
	for _, format := range []string{
		"yaml", "custom-columns", "custom-columns=", "custom-columns=,log", "custom-columns=timestamp,",
		"custom-columns=timestamp,,log", "custom-columns=metadata..podName", "custom-columns=.log",
		"custom-columns=metadata.", "custom-columns=metadata. podName", "custom-columns=log message",
	} {
		assert.Error(t, Validate(format), format)
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
			"2026-11-10T08:21:04.512Z INFO snip-api-service-development-6d8f7c9b5-x2k4p GET /api/v1/urls/abc123 302 1.4ms\n"},
		{"order and repeated fields", "custom-columns=level,log,level", "INFO GET /api/v1/urls/abc123 302 1.4ms INFO\n"},
		{"missing and null fields", "custom-columns=missing,metadata.missing,metadata.null,level.foo,log", "- - - - GET /api/v1/urls/abc123 302 1.4ms\n"},
		{"scalar values", "custom-columns=count,ready,empty,level", "9007199254740993 false  INFO\n"},
		{"trim field whitespace", "custom-columns= level , metadata.podName ", "INFO snip-api-service-development-6d8f7c9b5-x2k4p\n"},
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
