// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// ── deriveRunStatusFromMap ──────────────────────────────────────────

func TestDeriveRunStatusFromMap(t *testing.T) {
	tests := []struct {
		name    string
		reasons map[string]int
		want    string
	}{
		{"completed wins", map[string]int{"Completed": 1, "SuccessfulCreate": 1}, "succeeded"},
		{"backoff limit means failed", map[string]int{"BackoffLimitExceeded": 1, "SuccessfulCreate": 1}, "failed"},
		{"deadline means failed", map[string]int{"DeadlineExceeded": 1}, "failed"},
		{"failed create means failed", map[string]int{"FailedCreate": 1}, "failed"},
		{"only successful create means running", map[string]int{"SuccessfulCreate": 1}, "running"},
		{"empty means unknown", map[string]int{}, "unknown"},
		{"unrecognized reason means unknown", map[string]int{"Foo": 1}, "unknown"},
		{"completed beats failed", map[string]int{"Completed": 1, "BackoffLimitExceeded": 1}, "succeeded"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deriveRunStatusFromMap(tc.reasons))
		})
	}
}

// ── deriveFailureReasonFromMap ──────────────────────────────────────

func TestDeriveFailureReasonFromMap(t *testing.T) {
	tests := []struct {
		name    string
		reasons map[string]int
		want    string
	}{
		{"backoff limit", map[string]int{"BackoffLimitExceeded": 1}, "BackoffLimitExceeded"},
		{"deadline", map[string]int{"DeadlineExceeded": 1}, "DeadlineExceeded"},
		{"failed create", map[string]int{"FailedCreate": 1}, "FailedCreate"},
		{"backoff wins over deadline (ordered preference)", map[string]int{"DeadlineExceeded": 1, "BackoffLimitExceeded": 1}, "BackoffLimitExceeded"},
		{"no failure reason returns empty", map[string]int{"Completed": 1}, ""},
		{"empty returns empty", map[string]int{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deriveFailureReasonFromMap(tc.reasons))
		})
	}
}

// ── deriveRetryStatusFromMap ────────────────────────────────────────

func TestDeriveRetryStatusFromMap(t *testing.T) {
	tests := []struct {
		name    string
		reasons map[string]int
		want    string
	}{
		{"completed", map[string]int{"Completed": 1}, "Succeeded"},
		{"oom killed", map[string]int{"OOMKilled": 1}, "Failed"},
		{"crashloop", map[string]int{"CrashLoopBackOff": 1}, "Failed"},
		{"backoff", map[string]int{"BackOff": 1}, "Failed"},
		{"started means running", map[string]int{"Started": 1}, "Running"},
		{"pulled means running", map[string]int{"Pulled": 1}, "Running"},
		{"completed beats failed", map[string]int{"Completed": 1, "OOMKilled": 1}, "Succeeded"},
		{"failed beats running", map[string]int{"OOMKilled": 1, "Started": 1}, "Failed"},
		{"empty means unknown", map[string]int{}, "Unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deriveRetryStatusFromMap(tc.reasons))
		})
	}
}

// ── applyRunStatusOverride ──────────────────────────────────────────

func TestApplyRunStatusOverride(t *testing.T) {
	mk := func(statuses ...string) []types.RetryEntry {
		retries := make([]types.RetryEntry, len(statuses))
		for i, s := range statuses {
			retries[i] = types.RetryEntry{PodName: "p" + string(rune('1'+i)), Status: s}
		}
		return retries
	}

	t.Run("failed job marks every retry Failed", func(t *testing.T) {
		retries := mk("Running", "Running", "Running")
		applyRunStatusOverride(retries, "failed")
		for _, r := range retries {
			assert.Equal(t, "Failed", r.Status)
		}
	})

	t.Run("succeeded job marks last Succeeded and earlier ones Failed", func(t *testing.T) {
		retries := mk("Running", "Running", "Running")
		applyRunStatusOverride(retries, "succeeded")
		assert.Equal(t, "Failed", retries[0].Status)
		assert.Equal(t, "Failed", retries[1].Status)
		assert.Equal(t, "Succeeded", retries[2].Status)
	})

	t.Run("running job with multiple retries marks all but last Failed and preserves last", func(t *testing.T) {
		retries := mk("Running", "Running", "Running")
		applyRunStatusOverride(retries, "running")
		assert.Equal(t, "Failed", retries[0].Status)
		assert.Equal(t, "Failed", retries[1].Status)
		assert.Equal(t, "Running", retries[2].Status, "last retry should keep its derived status")
	})

	t.Run("unknown job leaves statuses untouched", func(t *testing.T) {
		retries := mk("Succeeded", "Failed", "Running")
		applyRunStatusOverride(retries, "unknown")
		assert.Equal(t, []string{"Succeeded", "Failed", "Running"}, []string{retries[0].Status, retries[1].Status, retries[2].Status})
	})

	t.Run("empty retries does not panic", func(t *testing.T) {
		var retries []types.RetryEntry
		applyRunStatusOverride(retries, "failed")
		assert.Empty(t, retries)
	})
}

// ── paginate ────────────────────────────────────────────────────────

func TestPaginate(t *testing.T) {
	mk := func(n int) []types.RunEntry {
		out := make([]types.RunEntry, n)
		for i := range out {
			out[i].JobName = string(rune('a' + i))
		}
		return out
	}

	tests := []struct {
		name     string
		input    []types.RunEntry
		offset   int
		limit    int
		wantLen  int
		wantHead string
	}{
		{"no limit returns everything from offset", mk(5), 0, 0, 5, "a"},
		{"limit truncates", mk(5), 0, 2, 2, "a"},
		{"offset skips", mk(5), 2, 2, 2, "c"},
		{"offset past end returns empty", mk(5), 10, 0, 0, ""},
		{"negative offset clamps to 0", mk(5), -1, 0, 5, "a"},
		{"limit larger than remaining returns remaining", mk(3), 1, 10, 2, "b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := paginate(tc.input, tc.offset, tc.limit)
			assert.Len(t, got, tc.wantLen)
			if tc.wantLen > 0 {
				assert.Equal(t, tc.wantHead, got[0].JobName)
			}
		})
	}
}

// ── sortRuns ─────────────────────────────────────────────────────────

func TestSortRuns(t *testing.T) {
	mk := func(ts ...string) []types.RunEntry {
		out := make([]types.RunEntry, len(ts))
		for i, t := range ts {
			out[i].StartTime = t
		}
		return out
	}

	t.Run("desc sorts newest first", func(t *testing.T) {
		runs := mk("2026-06-01T00:00:00Z", "2026-06-03T00:00:00Z", "2026-06-02T00:00:00Z")
		sortRuns(runs, "desc")
		assert.Equal(t, "2026-06-03T00:00:00Z", runs[0].StartTime)
		assert.Equal(t, "2026-06-01T00:00:00Z", runs[2].StartTime)
	})

	t.Run("asc sorts oldest first", func(t *testing.T) {
		runs := mk("2026-06-01T00:00:00Z", "2026-06-03T00:00:00Z", "2026-06-02T00:00:00Z")
		sortRuns(runs, "asc")
		assert.Equal(t, "2026-06-01T00:00:00Z", runs[0].StartTime)
		assert.Equal(t, "2026-06-03T00:00:00Z", runs[2].StartTime)
	})

	t.Run("empty input does not panic", func(t *testing.T) {
		var runs []types.RunEntry
		sortRuns(runs, "desc")
		assert.Empty(t, runs)
	})
}

// ── groupRuns ───────────────────────────────────────────────────────

func TestGroupRuns(t *testing.T) {
	ts := func(s string) time.Time {
		t, _ := time.Parse(time.RFC3339, s)
		return t
	}
	evt := func(kind, name, reason string, when time.Time) observability.EventEntry {
		return observability.EventEntry{
			ObjectKind: kind, ObjectName: name,
			Reason: reason, Message: reason + " happened",
			Type: "Normal", Timestamp: when,
		}
	}

	t.Run("non-Job events are ignored", func(t *testing.T) {
		runs := groupRuns([]observability.EventEntry{
			evt("Pod", "p-1", "Started", ts("2026-06-22T00:00:00Z")),
			evt("CronJob", "ct", "SawCompletedJob", ts("2026-06-22T00:00:00Z")),
		}, false)
		assert.Empty(t, runs)
	})

	t.Run("Job events without name are dropped", func(t *testing.T) {
		runs := groupRuns([]observability.EventEntry{evt("Job", "", "Completed", ts("2026-06-22T00:00:00Z"))}, false)
		assert.Empty(t, runs)
	})

	t.Run("succeeded run aggregates timestamps and counts", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Job", "job-a", "SuccessfulCreate", ts("2026-06-22T00:00:00Z")),
			evt("Job", "job-a", "Completed", ts("2026-06-22T00:00:30Z")),
		}
		runs := groupRuns(events, false)
		assert.Len(t, runs, 1)
		r := runs[0]
		assert.Equal(t, "job-a", r.JobName)
		assert.Equal(t, "succeeded", r.Status)
		assert.Equal(t, 2, r.EventCount)
		assert.Equal(t, "2026-06-22T00:00:00Z", r.StartTime)
		assert.Equal(t, "2026-06-22T00:00:30Z", r.CompletionTime)
		assert.Empty(t, r.Events, "events should be omitted when includeEvents=false")
		assert.Empty(t, r.FailureReason)
	})

	t.Run("failed run sets failureReason", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Job", "job-b", "SuccessfulCreate", ts("2026-06-22T00:00:00Z")),
			evt("Job", "job-b", "BackoffLimitExceeded", ts("2026-06-22T00:01:00Z")),
		}
		runs := groupRuns(events, false)
		assert.Len(t, runs, 1)
		assert.Equal(t, "failed", runs[0].Status)
		assert.Equal(t, "BackoffLimitExceeded", runs[0].FailureReason)
		assert.Equal(t, "2026-06-22T00:01:00Z", runs[0].CompletionTime)
	})

	t.Run("running run leaves completionTime empty", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Job", "job-c", "SuccessfulCreate", ts("2026-06-22T00:00:00Z")),
		}
		runs := groupRuns(events, false)
		assert.Len(t, runs, 1)
		assert.Equal(t, "running", runs[0].Status)
		assert.Empty(t, runs[0].CompletionTime)
	})

	t.Run("includeEvents emits per-run events sorted ascending", func(t *testing.T) {
		events := []observability.EventEntry{
			// Intentionally out of order to verify sort.
			evt("Job", "job-d", "Completed", ts("2026-06-22T00:00:30Z")),
			evt("Job", "job-d", "SuccessfulCreate", ts("2026-06-22T00:00:00Z")),
		}
		runs := groupRuns(events, true)
		assert.Len(t, runs, 1)
		assert.Len(t, runs[0].Events, 2)
		assert.Equal(t, "SuccessfulCreate", runs[0].Events[0].Reason)
		assert.Equal(t, "Completed", runs[0].Events[1].Reason)
	})
}

// ── groupRetries ────────────────────────────────────────────────────

func TestGroupRetries(t *testing.T) {
	ts := func(s string) time.Time {
		t, _ := time.Parse(time.RFC3339, s)
		return t
	}
	evt := func(kind, name, reason string, when time.Time) observability.EventEntry {
		return observability.EventEntry{
			ObjectKind: kind, ObjectName: name,
			Reason: reason, Message: reason, Type: "Normal", Timestamp: when,
		}
	}

	t.Run("pods belonging to the job become retries; others are ignored", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Job", "job-a", "SuccessfulCreate", ts("2026-06-22T00:00:00Z")),
			evt("Pod", "job-a-aaa", "Scheduled", ts("2026-06-22T00:00:05Z")),
			evt("Pod", "job-a-bbb", "Scheduled", ts("2026-06-22T00:00:10Z")),
			evt("Pod", "unrelated-ccc", "Started", ts("2026-06-22T00:00:15Z")),
		}
		retries, jobStatus := groupRetries(events, "job-a")
		assert.Equal(t, "running", jobStatus)
		assert.Len(t, retries, 2)
		assert.Equal(t, "job-a-aaa", retries[0].PodName)
		assert.Equal(t, "job-a-bbb", retries[1].PodName, "ordered by start time ascending")
	})

	t.Run("parent Job's events feed the returned job status", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Job", "job-a", "SuccessfulCreate", ts("2026-06-22T00:00:00Z")),
			evt("Job", "job-a", "Completed", ts("2026-06-22T00:01:00Z")),
			evt("Pod", "job-a-aaa", "Started", ts("2026-06-22T00:00:05Z")),
		}
		_, jobStatus := groupRetries(events, "job-a")
		assert.Equal(t, "succeeded", jobStatus)
	})

	t.Run("a different job's events do not pollute jobReasons", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Job", "job-a", "Completed", ts("2026-06-22T00:01:00Z")),
			evt("Job", "job-b", "BackoffLimitExceeded", ts("2026-06-22T00:01:00Z")),
			evt("Pod", "job-a-aaa", "Started", ts("2026-06-22T00:00:05Z")),
		}
		_, jobStatus := groupRetries(events, "job-a")
		assert.Equal(t, "succeeded", jobStatus, "job-b's failure must not bleed into job-a's status")
	})

	t.Run("events per retry are sorted ascending and counted", func(t *testing.T) {
		events := []observability.EventEntry{
			evt("Pod", "job-a-aaa", "Started", ts("2026-06-22T00:00:10Z")),
			evt("Pod", "job-a-aaa", "Scheduled", ts("2026-06-22T00:00:05Z")),
		}
		retries, _ := groupRetries(events, "job-a")
		assert.Len(t, retries, 1)
		assert.Equal(t, 2, retries[0].EventCount)
		assert.Equal(t, "Scheduled", retries[0].Events[0].Reason)
		assert.Equal(t, "Started", retries[0].Events[1].Reason)
	})

	t.Run("pod whose name does not start with jobName- is ignored", func(t *testing.T) {
		// job-a-extra would prefix-match "job-a-" so use a name that doesn't.
		events := []observability.EventEntry{
			evt("Pod", "different-pod-xxx", "Started", ts("2026-06-22T00:00:05Z")),
		}
		retries, _ := groupRetries(events, "job-a")
		assert.Empty(t, retries)
	})
}

// ── parseTimeRange ──────────────────────────────────────────────────

func TestParseTimeRange(t *testing.T) {
	t.Run("valid range returns parsed window", func(t *testing.T) {
		s, e, err := parseTimeRange("2026-06-22T00:00:00Z", "2026-06-22T01:00:00Z")
		assert.NoError(t, err)
		assert.Equal(t, "2026-06-22T00:00:00Z", s.UTC().Format(time.RFC3339))
		assert.Equal(t, "2026-06-22T01:00:00Z", e.UTC().Format(time.RFC3339))
	})
	t.Run("equal start and end is allowed (zero-length window)", func(t *testing.T) {
		_, _, err := parseTimeRange("2026-06-22T00:00:00Z", "2026-06-22T00:00:00Z")
		assert.NoError(t, err)
	})
	t.Run("inverted range returns error", func(t *testing.T) {
		_, _, err := parseTimeRange("2026-06-22T02:00:00Z", "2026-06-22T01:00:00Z")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "end time must be greater than or equal to start time")
	})
	t.Run("malformed start returns error", func(t *testing.T) {
		_, _, err := parseTimeRange("not-a-time", "2026-06-22T01:00:00Z")
		assert.Error(t, err)
	})
	t.Run("malformed end returns error", func(t *testing.T) {
		_, _, err := parseTimeRange("2026-06-22T00:00:00Z", "not-a-time")
		assert.Error(t, err)
	})
}

// ── parseOptionalTimeRange ──────────────────────────────────────────

func TestParseOptionalTimeRange(t *testing.T) {
	t.Run("both provided returns parsed window", func(t *testing.T) {
		s, e, err := parseOptionalTimeRange("2026-06-22T00:00:00Z", "2026-06-22T01:00:00Z")
		assert.NoError(t, err)
		assert.Equal(t, "2026-06-22T00:00:00Z", s.UTC().Format(time.RFC3339))
		assert.Equal(t, "2026-06-22T01:00:00Z", e.UTC().Format(time.RFC3339))
	})
	t.Run("both empty falls back to 30d window", func(t *testing.T) {
		s, e, err := parseOptionalTimeRange("", "")
		assert.NoError(t, err)
		// Fallback should be ~30 days; loose check to avoid flakiness.
		assert.InDelta(t, 30*24*time.Hour, e.Sub(s), float64(time.Minute))
	})
	t.Run("only startTime provided returns error", func(t *testing.T) {
		_, _, err := parseOptionalTimeRange("2026-06-22T00:00:00Z", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be provided together")
	})
	t.Run("only endTime provided returns error", func(t *testing.T) {
		_, _, err := parseOptionalTimeRange("", "2026-06-22T01:00:00Z")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be provided together")
	})
	t.Run("invalid time returns error", func(t *testing.T) {
		_, _, err := parseOptionalTimeRange("not-a-time", "2026-06-22T01:00:00Z")
		assert.Error(t, err)
	})
}
