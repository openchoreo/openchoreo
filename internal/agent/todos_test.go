// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTodos(t *testing.T) {
	t.Parallel()
	tool := NewWriteTodosTool()

	tests := []struct {
		name      string
		args      string
		wantErr   string
		wantParts []string // substrings that must appear in output
	}{
		{
			name: "all statuses",
			args: `{"todos":[{"content":"check logs","status":"completed"},{"content":"check metrics","status":"in_progress"},{"content":"write report","status":"pending"}]}`,
			wantParts: []string{
				"[x] check logs",
				"[~] check metrics",
				"[ ] write report",
				"1 pending",
				"1 in progress",
				"1 completed",
			},
		},
		{
			name: "empty list",
			args: `{"todos":[]}`,
			wantParts: []string{
				"0 pending",
				"0 in progress",
				"0 completed",
			},
		},
		{
			name:    "empty content",
			args:    `{"todos":[{"content":"  ","status":"pending"}]}`,
			wantErr: "todo content must not be empty",
		},
		{
			name:    "invalid status",
			args:    `{"todos":[{"content":"task","status":"done"}]}`,
			wantErr: "invalid status",
		},
		{
			name:    "missing todos field",
			args:    `{}`,
			wantErr: "missing required field: todos",
		},
		{
			name:    "null todos field",
			args:    `{"todos":null}`,
			wantErr: "missing required field: todos",
		},
		{
			name:    "invalid JSON",
			args:    `{bad}`,
			wantErr: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := tool.Execute(context.Background(), json.RawMessage(tt.args))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			for _, part := range tt.wantParts {
				assert.Contains(t, result, part)
			}
		})
	}
}
