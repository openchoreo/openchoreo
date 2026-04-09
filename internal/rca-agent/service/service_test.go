// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/agent"
)

func TestExtractSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		report json.RawMessage
		want   *string
	}{
		{
			name:   "valid summary",
			report: json.RawMessage(`{"summary":"high CPU usage","details":"..."}`),
			want:   strPtr("high CPU usage"),
		},
		{
			name:   "empty summary field",
			report: json.RawMessage(`{"summary":"","details":"..."}`),
			want:   nil,
		},
		{
			name:   "no summary key",
			report: json.RawMessage(`{"details":"something"}`),
			want:   nil,
		},
		{
			name:   "invalid JSON",
			report: json.RawMessage(`not json`),
			want:   nil,
		},
		{
			name:   "null report",
			report: nil,
			want:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractSummary(tc.report)
			if tc.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tc.want, *got)
			}
		})
	}
}

func TestFilterTools(t *testing.T) {
	t.Parallel()

	tools := []agent.Tool{
		{Name: "list_components"},
		{Name: "get_logs"},
		{Name: "get_traces"},
		{Name: "list_release_bindings"},
	}

	whitelist := map[string]bool{
		"list_components":       true,
		"list_release_bindings": true,
	}

	filtered := filterTools(tools, whitelist)

	assert.Len(t, filtered, 2)
	assert.Equal(t, "list_components", filtered[0].Name)
	assert.Equal(t, "list_release_bindings", filtered[1].Name)
}

func TestFilterTools_EmptyWhitelist(t *testing.T) {
	t.Parallel()

	tools := []agent.Tool{
		{Name: "list_components"},
		{Name: "get_logs"},
	}

	filtered := filterTools(tools, map[string]bool{})
	assert.Empty(t, filtered)
}

func TestFilterTools_NilTools(t *testing.T) {
	t.Parallel()

	filtered := filterTools(nil, map[string]bool{"a": true})
	assert.Empty(t, filtered)
}

func TestSendChatEvent_Success(t *testing.T) {
	t.Parallel()

	ch := make(chan ChatEvent, 1)
	ev := ChatEvent{Type: "text", Content: "hello"}

	ok := sendChatEvent(context.Background(), ch, ev)

	assert.True(t, ok)
	got := <-ch
	assert.Equal(t, ev, got)
}

func TestSendChatEvent_CancelledContext(t *testing.T) {
	t.Parallel()

	ch := make(chan ChatEvent) // unbuffered — will block
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok := sendChatEvent(ctx, ch, ChatEvent{Type: "text"})
	assert.False(t, ok)
}

func TestRandomHex_Length(t *testing.T) {
	t.Parallel()

	hex4 := randomHex(4)
	assert.Len(t, hex4, 8) // 4 bytes = 8 hex chars

	hex6 := randomHex(6)
	assert.Len(t, hex6, 12)
}

func TestRandomHex_Unique(t *testing.T) {
	t.Parallel()

	a := randomHex(8)
	b := randomHex(8)
	assert.NotEqual(t, a, b)
}

func strPtr(s string) *string {
	return &s
}
