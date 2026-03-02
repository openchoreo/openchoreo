// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const defaultPlaneName = "default"

// Logs fetches and displays logs for a workflow run
func (w *WorkflowRun) Logs(params LogsParams) error {
	if err := validation.ValidateParams(validation.CmdLogs, validation.ResourceWorkflowRun, params); err != nil {
		return err
	}

	// Validate --since early so both live and archived paths get consistent error handling
	if params.Since != "" {
		d, err := time.ParseDuration(params.Since)
		if err != nil {
			return fmt.Errorf("invalid --since value %q: %w", params.Since, err)
		}
		if d <= 0 {
			return fmt.Errorf("invalid --since value %q: duration must be positive", params.Since)
		}
	}

	ctx := context.Background()

	apiClient, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Get workflow run status to check if live logs are available
	status, err := apiClient.GetWorkflowRunStatus(ctx, params.Namespace, params.WorkflowRunName)
	if err != nil {
		return fmt.Errorf("failed to get workflow run status: %w", err)
	}

	if status.HasLiveObservability {
		return w.fetchLiveLogs(ctx, apiClient, params)
	}

	return w.fetchArchivedLogs(ctx, apiClient, params)
}

// fetchLiveLogs fetches logs from the OpenChoreo API (build plane proxy)
func (w *WorkflowRun) fetchLiveLogs(ctx context.Context, apiClient *client.Client, params LogsParams) error {
	sinceSeconds := parseSinceToSeconds(params.Since)

	if params.Follow {
		return w.followLiveLogs(ctx, apiClient, params, sinceSeconds)
	}

	logParams := &gen.GetWorkflowRunLogsParams{}
	if sinceSeconds > 0 {
		logParams.SinceSeconds = &sinceSeconds
	}

	entries, err := apiClient.GetWorkflowRunLogs(ctx, params.Namespace, params.WorkflowRunName, logParams)
	if err != nil {
		return fmt.Errorf("failed to get live logs: %w", err)
	}

	printLogEntries(entries)
	return nil
}

// followLiveLogs continuously polls for new live logs
func (w *WorkflowRun) followLiveLogs(ctx context.Context, apiClient *client.Client, params LogsParams, sinceSeconds int64) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initial fetch
	logParams := &gen.GetWorkflowRunLogsParams{}
	if sinceSeconds > 0 {
		logParams.SinceSeconds = &sinceSeconds
	}

	entries, err := apiClient.GetWorkflowRunLogs(ctx, params.Namespace, params.WorkflowRunName, logParams)
	if err != nil {
		return fmt.Errorf("failed to get live logs: %w", err)
	}
	printLogEntries(entries)

	// Track the last-seen timestamp to avoid printing duplicates
	var lastSeen time.Time
	if len(entries) > 0 {
		if ts := entries[len(entries)-1].Timestamp; ts != nil {
			lastSeen = *ts
		}
	}

	// Poll with a window large enough to not miss logs, deduplicate client-side
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	pollInterval := int64(5)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopping log streaming...")
			return nil
		case <-ticker.C:
			// Check if the run still has live observability
			status, err := apiClient.GetWorkflowRunStatus(ctx, params.Namespace, params.WorkflowRunName)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				fmt.Fprintf(os.Stderr, "Error checking workflow run status: %v\n", err)
				continue
			}

			if !status.HasLiveObservability {
				fmt.Println("\nWorkflow run completed. Live logs are no longer available.")
				return nil
			}

			p := &gen.GetWorkflowRunLogsParams{
				SinceSeconds: &pollInterval,
			}
			entries, err := apiClient.GetWorkflowRunLogs(ctx, params.Namespace, params.WorkflowRunName, p)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				fmt.Fprintf(os.Stderr, "Error fetching logs: %v\n", err)
				continue
			}

			// Filter out entries already printed
			newEntries := filterNewEntries(entries, lastSeen)
			printLogEntries(newEntries)

			// Advance the cursor
			if len(newEntries) > 0 {
				if ts := newEntries[len(newEntries)-1].Timestamp; ts != nil {
					lastSeen = *ts
				}
			}
		}
	}
}

// filterNewEntries returns only log entries whose timestamp is strictly after lastSeen.
func filterNewEntries(entries []gen.WorkflowRunLogEntry, lastSeen time.Time) []gen.WorkflowRunLogEntry {
	if lastSeen.IsZero() {
		return entries
	}
	var filtered []gen.WorkflowRunLogEntry
	for _, e := range entries {
		if e.Timestamp != nil && e.Timestamp.After(lastSeen) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// fetchArchivedLogs fetches logs from the observer (OpenSearch)
func (w *WorkflowRun) fetchArchivedLogs(ctx context.Context, apiClient *client.Client, params LogsParams) error {
	// Get the workflow run to find its workflow name and UID
	workflowRun, err := apiClient.GetWorkflowRun(ctx, params.Namespace, params.WorkflowRunName)
	if err != nil {
		return fmt.Errorf("failed to get workflow run: %w", err)
	}

	workflowName := ""
	if workflowRun.Spec != nil {
		workflowName = workflowRun.Spec.Workflow.Name
	}
	if workflowName == "" {
		return fmt.Errorf("workflow run %s has no workflow reference", params.WorkflowRunName)
	}

	// Resolve the observer URL from the build plane chain
	observerURL, err := resolveObserverURL(ctx, apiClient, params.Namespace, workflowName)
	if err != nil {
		return fmt.Errorf("failed to resolve observer URL: %w", err)
	}

	// Get credential for observer API auth
	credential, err := config.GetCurrentCredential()
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("no current credential available")
	}

	// Calculate time range
	since := params.Since
	if since == "" {
		since = "720h" // default 30 days for archived logs
	}
	duration, err := time.ParseDuration(since)
	if err != nil {
		return fmt.Errorf("invalid duration format for --since: %w", err)
	}

	startTime := time.Now().Add(-duration)
	endTime := time.Now()

	// Use the workflow run name as the run ID for the observer query
	runID := params.WorkflowRunName
	if workflowRun.Metadata.Uid != nil && *workflowRun.Metadata.Uid != "" {
		runID = *workflowRun.Metadata.Uid
	}

	obsClient := client.NewObserverClient(observerURL, credential.Token)
	logResponse, err := obsClient.FetchWorkflowRunLogs(ctx, runID, client.WorkflowRunLogsRequest{
		NamespaceName: params.Namespace,
		StartTime:     startTime.Format(time.RFC3339),
		EndTime:       endTime.Format(time.RFC3339),
		SortOrder:     "asc",
	})
	if err != nil {
		return fmt.Errorf("failed to fetch archived logs from observer %s: %w", observerURL, err)
	}

	if len(logResponse.Logs) == 0 {
		fmt.Println("No logs found for this workflow run")
		return nil
	}

	for _, log := range logResponse.Logs {
		fmt.Printf("%s %s\n", log.Timestamp, log.Log)
	}

	if params.Follow {
		fmt.Println("\nNote: Follow mode is not available for archived logs (workflow run has already completed)")
	}

	return nil
}

// resolveObserverURL resolves the observer URL by traversing:
// Workflow.BuildPlaneRef -> BuildPlane/ClusterBuildPlane -> ObservabilityPlane/ClusterObservabilityPlane -> ObserverURL
// Falls back to default build planes if the workflow has no buildPlaneRef.
func resolveObserverURL(ctx context.Context, apiClient *client.Client, namespace, workflowName string) (string, error) {
	// Resolve build plane - try workflow's buildPlaneRef first, then fall back to defaults
	obsPlaneRef, clusterObsPlaneRef := resolveBuildPlaneObsRef(ctx, apiClient, namespace, workflowName)

	// Resolve observer URL from the observability plane
	return resolveObserverURLFromObsRef(ctx, apiClient, namespace, obsPlaneRef, clusterObsPlaneRef)
}

// resolveBuildPlaneObsRef resolves the build plane and returns its observability plane reference.
// Resolution order:
// 1. Workflow's explicit buildPlaneRef (BuildPlane or ClusterBuildPlane)
// 2. Namespaced BuildPlane named "default"
// 3. ClusterBuildPlane named "default"
func resolveBuildPlaneObsRef(ctx context.Context, apiClient *client.Client, namespace, workflowName string) (*gen.ObservabilityPlaneRef, *gen.ClusterObservabilityPlaneRef) {
	// Try the workflow's explicit buildPlaneRef first
	if workflowName != "" {
		wf, err := apiClient.GetWorkflow(ctx, namespace, workflowName)
		if err == nil && wf.Spec != nil && wf.Spec.BuildPlaneRef != nil {
			ref := wf.Spec.BuildPlaneRef
			switch ref.Kind {
			case gen.BuildPlaneRefKindBuildPlane:
				bp, err := apiClient.GetBuildPlane(ctx, namespace, ref.Name)
				if err == nil && bp.Spec != nil && bp.Spec.ObservabilityPlaneRef != nil {
					return bp.Spec.ObservabilityPlaneRef, nil
				}
			case gen.BuildPlaneRefKindClusterBuildPlane:
				cbp, err := apiClient.GetClusterBuildPlane(ctx, ref.Name)
				if err == nil && cbp.Spec != nil && cbp.Spec.ObservabilityPlaneRef != nil {
					return nil, cbp.Spec.ObservabilityPlaneRef
				}
			}
		}
	}

	// Fall back to namespaced BuildPlane "default"
	bp, err := apiClient.GetBuildPlane(ctx, namespace, defaultPlaneName)
	if err == nil && bp.Spec != nil && bp.Spec.ObservabilityPlaneRef != nil {
		return bp.Spec.ObservabilityPlaneRef, nil
	}

	// Fall back to ClusterBuildPlane "default"
	cbp, err := apiClient.GetClusterBuildPlane(ctx, defaultPlaneName)
	if err == nil && cbp.Spec != nil && cbp.Spec.ObservabilityPlaneRef != nil {
		return nil, cbp.Spec.ObservabilityPlaneRef
	}

	// No build plane found - try default observability plane directly
	return nil, nil
}

// resolveObserverURLFromObsRef resolves the observer URL from observability plane references
func resolveObserverURLFromObsRef(ctx context.Context, apiClient *client.Client, namespace string, obsRef *gen.ObservabilityPlaneRef, clusterObsRef *gen.ClusterObservabilityPlaneRef) (string, error) {
	// If we have a namespaced observability plane ref
	if obsRef != nil {
		switch obsRef.Kind {
		case gen.ObservabilityPlaneRefKindObservabilityPlane:
			op, err := apiClient.GetObservabilityPlane(ctx, namespace, obsRef.Name)
			if err != nil {
				return "", fmt.Errorf("failed to get observability plane %s: %w", obsRef.Name, err)
			}
			if op.Spec != nil && op.Spec.ObserverURL != nil {
				return *op.Spec.ObserverURL, nil
			}
		case gen.ObservabilityPlaneRefKindClusterObservabilityPlane:
			cop, err := apiClient.GetClusterObservabilityPlane(ctx, obsRef.Name)
			if err != nil {
				return "", fmt.Errorf("failed to get cluster observability plane %s: %w", obsRef.Name, err)
			}
			if cop.Spec != nil && cop.Spec.ObserverURL != nil {
				return *cop.Spec.ObserverURL, nil
			}
		}
	}

	// If we have a cluster observability plane ref
	if clusterObsRef != nil {
		cop, err := apiClient.GetClusterObservabilityPlane(ctx, clusterObsRef.Name)
		if err != nil {
			return "", fmt.Errorf("failed to get cluster observability plane %s: %w", clusterObsRef.Name, err)
		}
		if cop.Spec != nil && cop.Spec.ObserverURL != nil {
			return *cop.Spec.ObserverURL, nil
		}
	}

	// Fallback: try default ObservabilityPlane in namespace, then default ClusterObservabilityPlane
	op, err := apiClient.GetObservabilityPlane(ctx, namespace, defaultPlaneName)
	if err == nil && op.Spec != nil && op.Spec.ObserverURL != nil {
		return *op.Spec.ObserverURL, nil
	}

	cop, err := apiClient.GetClusterObservabilityPlane(ctx, defaultPlaneName)
	if err == nil && cop.Spec != nil && cop.Spec.ObserverURL != nil {
		return *cop.Spec.ObserverURL, nil
	}

	return "", fmt.Errorf("no observer URL configured: could not find an observability plane with a configured observer URL")
}

// printLogEntries prints workflow run log entries to stdout
func printLogEntries(entries []gen.WorkflowRunLogEntry) {
	for _, entry := range entries {
		if entry.Timestamp != nil {
			fmt.Printf("%s %s\n", entry.Timestamp.Format(time.RFC3339), entry.Log)
		} else {
			fmt.Println(entry.Log)
		}
	}
}

// parseSinceToSeconds converts a duration string (e.g., "5m", "1h") to seconds
func parseSinceToSeconds(since string) int64 {
	if since == "" {
		return 0
	}
	d, err := time.ParseDuration(since)
	if err != nil {
		return 0
	}
	return int64(d.Seconds())
}
