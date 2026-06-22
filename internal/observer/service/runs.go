// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// adapterEventLimit is the per-call cap on events fetched from the logs adapter.
// 1000 is the adapter's documented maximum; for scheduled-task observability over
// a reasonable time window this is comfortably above typical event counts.
const adapterEventLimit = 1000

// RunsService groups Kubernetes events into scheduled-task runs (Jobs) and retries (Pods).
//
// It fetches events from the upstream logs adapter (no direct OpenSearch access) and
// groups them in-memory by involvedObject. Status is derived from event reasons
// (see deriveRunStatus / deriveRetryStatus). Pod-level Succeeded/Failed events
// are not native K8s events, so applyRunStatusOverride uses the parent Job's outcome
// to fix up per-retry status for finished runs.
type RunsService struct {
	eventsAdapter observability.EventsAdapter
	config        *config.Config
	resolver      *ResourceUIDResolver
	logger        *slog.Logger
}

var (
	// ErrRunsResolveSearchScope indicates a failure while resolving scope/resource identifiers.
	ErrRunsResolveSearchScope = errors.New("runs search scope resolution failed")
	// ErrRunsRetrieval indicates a failure while retrieving events from the adapter.
	ErrRunsRetrieval = errors.New("runs retrieval failed")
)

// NewRunsService creates a new RunsService backed by the HTTP logs adapter.
func NewRunsService(
	eventsAdapter observability.EventsAdapter,
	resolver *ResourceUIDResolver,
	cfg *config.Config,
	logger *slog.Logger,
) (*RunsService, error) {
	if eventsAdapter == nil {
		return nil, fmt.Errorf("events adapter is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RunsService{
		eventsAdapter: eventsAdapter,
		config:        cfg,
		resolver:      resolver,
		logger:        logger,
	}, nil
}

// QueryRuns lists runs (Jobs) for a scheduled task component over the given time window.
func (s *RunsService) QueryRuns(ctx context.Context, req *types.RunsQueryRequest) (*types.RunsQueryResponse, error) {
	if req == nil || req.SearchScope == nil {
		return nil, fmt.Errorf("request and searchScope are required")
	}

	s.logger.Debug("QueryRuns called",
		"namespace", req.SearchScope.Namespace,
		"component", req.SearchScope.Component,
		"environment", req.SearchScope.Environment,
		"startTime", req.StartTime,
		"endTime", req.EndTime)

	componentScope, err := s.resolveComponentScope(ctx, req.SearchScope)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRunsResolveSearchScope, err)
	}

	startTime, endTime, err := parseTimeRange(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}

	events, took, err := s.fetchComponentEvents(ctx, componentScope, startTime, endTime, req.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRunsRetrieval, err)
	}

	runs := groupRuns(events, req.IncludeEvents)
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	sortRuns(runs, sortOrder)

	total := len(runs)
	runs = paginate(runs, req.Offset, req.Limit)

	return &types.RunsQueryResponse{
		Runs:   runs,
		Total:  total,
		TookMs: took,
	}, nil
}

// QueryRetries lists retries (Pods) for a specific run (Job).
func (s *RunsService) QueryRetries(ctx context.Context, jobName string, req *types.RetriesQueryRequest) (*types.RetriesQueryResponse, error) {
	if req == nil || req.SearchScope == nil {
		return nil, fmt.Errorf("request and searchScope are required")
	}
	if jobName == "" {
		return nil, fmt.Errorf("jobName is required")
	}

	s.logger.Debug("QueryRetries called",
		"jobName", jobName,
		"namespace", req.SearchScope.Namespace)

	componentScope, err := s.resolveComponentScope(ctx, req.SearchScope)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRunsResolveSearchScope, err)
	}

	// Use the request's window if provided; otherwise fall back to a wide 30-day
	// lookback. A narrow window scoped to the run's lifetime avoids the adapter's
	// per-call event cap truncating retries for high-frequency CronJobs.
	startTime, endTime, err := parseOptionalTimeRange(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}

	events, took, err := s.fetchComponentEvents(ctx, componentScope, startTime, endTime, "asc")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRunsRetrieval, err)
	}

	retries, jobStatus := groupRetries(events, jobName)
	applyRunStatusOverride(retries, jobStatus)

	return &types.RetriesQueryResponse{
		Retries: retries,
		Total:   len(retries),
		TookMs:  took,
	}, nil
}

// resolveComponentScope returns a fully resolved (UIDs present) component scope.
func (s *RunsService) resolveComponentScope(
	ctx context.Context,
	scope *types.ComponentSearchScope,
) (*internalSearchScope, error) {
	return resolveSearchScope(ctx, s.resolver, &types.SearchScope{Component: scope})
}

// fetchComponentEvents pulls events for the component from the logs adapter.
func (s *RunsService) fetchComponentEvents(
	ctx context.Context,
	scope *internalSearchScope,
	startTime, endTime time.Time,
	sortOrder string,
) ([]observability.EventEntry, int, error) {
	if sortOrder == "" {
		sortOrder = "desc"
	}
	params := observability.ComponentEventsParams{
		ComponentID:   scope.ComponentUID,
		EnvironmentID: scope.EnvironmentUID,
		ProjectID:     scope.ProjectUID,
		Namespace:     scope.NamespaceName,
		StartTime:     startTime,
		EndTime:       endTime,
		Limit:         adapterEventLimit,
		SortOrder:     sortOrder,
	}
	result, err := s.eventsAdapter.GetComponentEvents(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get component events from adapter: %w", err)
	}
	if result == nil {
		return nil, 0, fmt.Errorf("component events adapter returned nil result")
	}
	return result.Events, result.Took, nil
}

// parseTimeRange validates and parses required RFC3339 startTime / endTime.
func parseTimeRange(startStr, endStr string) (time.Time, time.Time, error) {
	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse start time: %w", err)
	}
	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse end time: %w", err)
	}
	return startTime, endTime, nil
}

// parseOptionalTimeRange parses RFC3339 startTime / endTime when provided, and
// falls back to a 30-day lookback ending at now when either is empty.
func parseOptionalTimeRange(startStr, endStr string) (time.Time, time.Time, error) {
	if startStr == "" || endStr == "" {
		endTime := time.Now().UTC()
		return endTime.Add(-30 * 24 * time.Hour), endTime, nil
	}
	return parseTimeRange(startStr, endStr)
}

// groupRuns groups Job-kind events by Job name into RunEntry list.
func groupRuns(events []observability.EventEntry, includeEvents bool) []types.RunEntry {
	type runState struct {
		earliestTS time.Time
		latestTS   time.Time
		eventCount int
		reasons    map[string]int
		events     []types.RunEvent
	}
	byName := make(map[string]*runState)

	for _, e := range events {
		if e.ObjectKind != "Job" {
			continue
		}
		name := e.ObjectName
		if name == "" {
			continue
		}
		st, ok := byName[name]
		if !ok {
			st = &runState{reasons: make(map[string]int)}
			byName[name] = st
		}
		st.eventCount++
		st.reasons[e.Reason]++
		if st.earliestTS.IsZero() || e.Timestamp.Before(st.earliestTS) {
			st.earliestTS = e.Timestamp
		}
		if e.Timestamp.After(st.latestTS) {
			st.latestTS = e.Timestamp
		}
		if includeEvents {
			st.events = append(st.events, types.RunEvent{
				Reason:    e.Reason,
				Message:   e.Message,
				Timestamp: e.Timestamp.UTC().Format(time.RFC3339),
				Type:      e.Type,
			})
		}
	}

	out := make([]types.RunEntry, 0, len(byName))
	for name, st := range byName {
		status := deriveRunStatusFromMap(st.reasons)
		run := types.RunEntry{
			JobName:    name,
			Status:     status,
			StartTime:  st.earliestTS.UTC().Format(time.RFC3339),
			EventCount: st.eventCount,
		}
		if !st.latestTS.IsZero() && (status == "succeeded" || status == "failed") {
			run.CompletionTime = st.latestTS.UTC().Format(time.RFC3339)
		}
		if status == "failed" {
			run.FailureReason = deriveFailureReasonFromMap(st.reasons)
		}
		if includeEvents {
			// Event list should be ascending for readability.
			sort.SliceStable(st.events, func(i, j int) bool {
				return st.events[i].Timestamp < st.events[j].Timestamp
			})
			run.Events = st.events
		}
		out = append(out, run)
	}
	return out
}

// groupRetries groups Pod-kind events whose pod name belongs to the given Job into
// RetryEntry list. Returns the parent Job's status alongside (needed for status override).
func groupRetries(events []observability.EventEntry, jobName string) ([]types.RetryEntry, string) {
	type retryState struct {
		earliestTS time.Time
		eventCount int
		reasons    map[string]int
		events     []types.RetryEvent
	}
	byPod := make(map[string]*retryState)
	jobReasons := make(map[string]int)
	podPrefix := jobName + "-"

	for _, e := range events {
		// Parent Job's events drive the run-level status used by the status override.
		if e.ObjectKind == "Job" && e.ObjectName == jobName {
			jobReasons[e.Reason]++
			continue
		}
		if e.ObjectKind != "Pod" {
			continue
		}
		if !strings.HasPrefix(e.ObjectName, podPrefix) {
			continue
		}
		name := e.ObjectName
		st, ok := byPod[name]
		if !ok {
			st = &retryState{reasons: make(map[string]int)}
			byPod[name] = st
		}
		st.eventCount++
		st.reasons[e.Reason]++
		if st.earliestTS.IsZero() || e.Timestamp.Before(st.earliestTS) {
			st.earliestTS = e.Timestamp
		}
		st.events = append(st.events, types.RetryEvent{
			Reason:    e.Reason,
			Message:   e.Message,
			Timestamp: e.Timestamp.UTC().Format(time.RFC3339),
			Type:      e.Type,
		})
	}

	retries := make([]types.RetryEntry, 0, len(byPod))
	for name, st := range byPod {
		sort.SliceStable(st.events, func(i, j int) bool {
			return st.events[i].Timestamp < st.events[j].Timestamp
		})
		retries = append(retries, types.RetryEntry{
			PodName:    name,
			Status:     deriveRetryStatusFromMap(st.reasons),
			StartTime:  st.earliestTS.UTC().Format(time.RFC3339),
			EventCount: st.eventCount,
			Events:     st.events,
		})
	}
	// Retries are listed by first_seen ascending (so the "last retry" used by the
	// status-override logic is well-defined as the final element).
	sort.SliceStable(retries, func(i, j int) bool {
		return retries[i].StartTime < retries[j].StartTime
	})

	return retries, deriveRunStatusFromMap(jobReasons)
}

// sortRuns orders runs by StartTime in the requested order. Stable so that runs with
// identical timestamps keep map-iteration order (rare with K8s ms-precision timestamps).
func sortRuns(runs []types.RunEntry, sortOrder string) {
	if sortOrder == "asc" {
		sort.SliceStable(runs, func(i, j int) bool {
			return runs[i].StartTime < runs[j].StartTime
		})
		return
	}
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].StartTime > runs[j].StartTime
	})
}

// paginate returns the slice between offset and offset+limit, with the same
// "limit <= 0 means no limit" convention used by the prior implementation.
func paginate(runs []types.RunEntry, offset, limit int) []types.RunEntry {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(runs) {
		return []types.RunEntry{}
	}
	runs = runs[offset:]
	if limit > 0 && limit < len(runs) {
		runs = runs[:limit]
	}
	return runs
}

// applyRunStatusOverride mutates retries in-place so their statuses reflect the parent
// Job's outcome. Retries are expected to be ordered by start_time ascending.
//
// Why this override exists: native K8s does not emit a Pod-level "Succeeded" / "Failed"
// event on container exit, so deriveRetryStatus from pod events alone reports "Running"
// forever for finished pods. We use the parent Job's terminal status to fix this up.
//
//   - Job failed: every retry is Failed.
//   - Job succeeded: the last retry succeeded; any earlier ones are by definition the
//     reason the Job retried, so they are Failed.
//   - Job running with 2+ retries: under restartPolicy: Never the Job controller only
//     spawns a new pod when the previous one failed, so all but the last retry are Failed.
//     The last retry keeps whatever deriveRetryStatus produced (it may still be running).
//   - Job unknown / running with a single retry: leave deriveRetryStatus output alone.
func applyRunStatusOverride(retries []types.RetryEntry, jobStatus string) {
	if len(retries) == 0 {
		return
	}
	switch jobStatus {
	case "failed":
		for i := range retries {
			retries[i].Status = "Failed"
		}
	case "succeeded":
		for i := range retries {
			if i == len(retries)-1 {
				retries[i].Status = "Succeeded"
			} else {
				retries[i].Status = "Failed"
			}
		}
	case "running":
		for i := 0; i < len(retries)-1; i++ {
			retries[i].Status = "Failed"
		}
	}
}

// deriveRunStatusFromMap determines run status from a reason-frequency map.
func deriveRunStatusFromMap(reasons map[string]int) string {
	if _, ok := reasons["Completed"]; ok {
		return "succeeded"
	}
	if _, ok := reasons["BackoffLimitExceeded"]; ok {
		return "failed"
	}
	if _, ok := reasons["DeadlineExceeded"]; ok {
		return "failed"
	}
	if _, ok := reasons["FailedCreate"]; ok {
		return "failed"
	}
	if _, ok := reasons["SuccessfulCreate"]; ok {
		return "running"
	}
	return "unknown"
}

// deriveFailureReasonFromMap returns the K8s event reason that caused the Job to fail,
// or "" if no failure-indicating reason is present. Reuses the same reasons map as
// deriveRunStatusFromMap, so it costs nothing extra.
func deriveFailureReasonFromMap(reasons map[string]int) string {
	for _, key := range []string{"BackoffLimitExceeded", "DeadlineExceeded", "FailedCreate"} {
		if _, ok := reasons[key]; ok {
			return key
		}
	}
	return ""
}

// deriveRetryStatusFromMap determines retry (Pod) status from pod-event reasons.
// Pod-level Succeeded/Failed events do not exist natively, so this is a best-effort
// guess from kubelet/scheduler events. applyRunStatusOverride corrects it using the
// parent Job's terminal status.
func deriveRetryStatusFromMap(reasons map[string]int) string {
	if _, ok := reasons["Completed"]; ok {
		return "Succeeded"
	}
	for _, key := range []string{"OOMKilled", "CrashLoopBackOff", "BackOff"} {
		if _, ok := reasons[key]; ok {
			return "Failed"
		}
	}
	for _, key := range []string{"Started", "Pulled"} {
		if _, ok := reasons[key]; ok {
			return "Running"
		}
	}
	return "Unknown"
}
