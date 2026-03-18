// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestFilterNewEntries(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)

	entry := func(ts time.Time, log string) gen.WorkflowRunLogEntry {
		return gen.WorkflowRunLogEntry{Timestamp: &ts, Log: log}
	}

	tests := []struct {
		name      string
		entries   []gen.WorkflowRunLogEntry
		lastSeen  time.Time
		wantCount int
		wantLogs  []string
	}{
		{
			name:      "zero lastSeen returns all",
			entries:   []gen.WorkflowRunLogEntry{entry(now, "a"), entry(later, "b")},
			lastSeen:  time.Time{},
			wantCount: 2,
			wantLogs:  []string{"a", "b"},
		},
		{
			name:      "filters entries before lastSeen",
			entries:   []gen.WorkflowRunLogEntry{entry(earlier, "old"), entry(now, "current"), entry(later, "new")},
			lastSeen:  now,
			wantCount: 1,
			wantLogs:  []string{"new"},
		},
		{
			name:      "all entries before lastSeen",
			entries:   []gen.WorkflowRunLogEntry{entry(earlier, "old")},
			lastSeen:  now,
			wantCount: 0,
		},
		{
			name:      "empty entries",
			entries:   nil,
			lastSeen:  now,
			wantCount: 0,
		},
		{
			name:      "entry with nil timestamp is skipped",
			entries:   []gen.WorkflowRunLogEntry{{Log: "no-ts"}, entry(later, "new")},
			lastSeen:  now,
			wantCount: 1,
			wantLogs:  []string{"new"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterNewEntries(tt.entries, tt.lastSeen)
			require.Len(t, got, tt.wantCount)
			for i, log := range tt.wantLogs {
				assert.Equal(t, log, got[i].Log)
			}
		})
	}
}

func TestParseSinceToSeconds(t *testing.T) {
	tests := []struct {
		name  string
		since string
		want  int64
	}{
		{name: "empty string", since: "", want: 0},
		{name: "5 minutes", since: "5m", want: 300},
		{name: "1 hour", since: "1h", want: 3600},
		{name: "30 seconds", since: "30s", want: 30},
		{name: "invalid returns 0", since: "notaduration", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseSinceToSeconds(tt.since))
		})
	}
}
