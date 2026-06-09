// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const defaultPlaneName = "default"

// Logs fetches and displays logs for a component
func (cp *Component) Logs(params LogsParams) error {
	ctx := context.Background()

	apiClient := cp.client

	// Verify the component exists
	if _, err := apiClient.GetComponent(ctx, params.Namespace, params.Component); err != nil {
		return fmt.Errorf("failed to get component: %w", err)
	}

	// If environment not specified, get the lowest environment from deployment pipeline
	envName := params.Environment
	if envName == "" {
		pipeline, err := apiClient.GetProjectDeploymentPipeline(ctx, params.Namespace, params.Project)
		if err != nil {
			return fmt.Errorf("failed to get deployment pipeline: %w", err)
		}

		// Find root environment (lowest environment)
		rootEnv, err := findRootEnvironment(pipeline)
		if err != nil {
			return fmt.Errorf("failed to find root environment: %w", err)
		}

		envName = rootEnv
	}

	// Resolve observer URL by traversing Environment → DataPlane → ObservabilityPlane
	observerURL, err := resolveObserverURL(ctx, apiClient, params.Namespace, envName)
	if err != nil {
		return fmt.Errorf("failed to resolve observer URL: %w", err)
	}

	// Get environment to resolve UID
	environment, err := apiClient.GetEnvironment(ctx, params.Namespace, envName)
	if err != nil {
		return fmt.Errorf("failed to get environment: %w", err)
	}

	// Update params with resolved environment name
	params.Environment = envName

	// Set defaults
	if params.Since == "" {
		params.Since = "1h"
	}

	// Calculate time range from --since flag
	duration, err := time.ParseDuration(params.Since)
	if err != nil {
		return fmt.Errorf("invalid duration format: %w", err)
	}

	startTime := time.Now().Add(-duration)
	endTime := time.Now()

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("no current credential available")
	}

	environmentUID := ""
	if environment.Metadata.Uid != nil {
		environmentUID = *environment.Metadata.Uid
	}

	if params.Follow {
		return cp.followLogs(ctx, observerURL, credential.Token, environmentUID, params, startTime, endTime)
	}

	return cp.fetchAndPrintLogs(ctx, observerURL, credential.Token, environmentUID, params, startTime, endTime)
}

// fetchAndPrintLogs fetches logs for a given time range and prints them
func (cp *Component) fetchAndPrintLogs(
	ctx context.Context,
	observerURL string,
	token string,
	environmentID string,
	params LogsParams,
	startTime time.Time,
	endTime time.Time,
) error {
	logs, err := cp.fetchLogs(ctx, observerURL, token, environmentID, params, startTime, endTime)
	if err != nil {
		return err
	}

	// When --tail is used, logs are fetched in desc order; reverse for chronological display
	if params.Tail > 0 {
		reverseLogs(logs)
	}

	for _, log := range logs {
		fmt.Printf("%s %s\n", log.Timestamp, log.Log)
	}

	return nil
}

// reverseLogs reverses a slice of log entries in place
func reverseLogs(logs []client.LogEntry) {
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
}

// followLogs continuously fetches and prints new logs
func (cp *Component) followLogs(
	ctx context.Context,
	observerURL string,
	token string,
	environmentID string,
	params LogsParams,
	startTime time.Time,
	endTime time.Time,
) error {
	// Set up signal handling for graceful shutdown with cancelable context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initial fetch (respects --tail for the initial batch)
	logs, err := cp.fetchLogs(ctx, observerURL, token, environmentID, params, startTime, endTime)
	if err != nil {
		return err
	}

	// When --tail is used, initial logs are fetched in desc order; reverse for chronological display
	if params.Tail > 0 {
		reverseLogs(logs)
	}

	// Print initial logs
	for _, log := range logs {
		fmt.Printf("%s %s\n", log.Timestamp, log.Log)
	}

	// Update startTime to the last log timestamp or endTime
	startTime = endTime // default
	if len(logs) > 0 {
		lastTimestamp, err := time.Parse(time.RFC3339, logs[len(logs)-1].Timestamp)
		if err == nil {
			startTime = lastTimestamp.Add(1 * time.Millisecond) // Add 1ms to avoid duplicate
		}
	}

	// Clear tail for subsequent polls — fetch all new logs in ascending order
	params.Tail = 0

	// Poll for new logs
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopping log streaming...")
			return nil
		case <-ticker.C:
			endTime = time.Now()

			logs, err := cp.fetchLogs(ctx, observerURL, token, environmentID, params, startTime, endTime)
			if err != nil {
				// Check if context was cancelled
				if ctx.Err() != nil {
					return nil
				}
				// Log error but continue
				fmt.Fprintf(os.Stderr, "Error fetching logs: %v\n", err)
				continue
			}

			// Print new logs
			for _, log := range logs {
				fmt.Printf("%s %s\n", log.Timestamp, log.Log)
			}

			// Update startTime
			if len(logs) > 0 {
				lastTimestamp, err := time.Parse(time.RFC3339, logs[len(logs)-1].Timestamp)
				if err == nil {
					startTime = lastTimestamp.Add(1 * time.Millisecond)
				} else {
					startTime = endTime
				}
			} else {
				startTime = endTime
			}
		}
	}
}

// fetchLogs makes an HTTP request to the observer to fetch logs
func (cp *Component) fetchLogs(
	ctx context.Context,
	observerURL string,
	token string,
	environmentID string,
	params LogsParams,
	startTime time.Time,
	endTime time.Time,
) ([]client.LogEntry, error) {
	sortOrder := "asc"
	if params.Tail > 0 {
		sortOrder = "desc"
	}

	reqBody := client.ComponentLogsRequest{
		StartTime:       startTime.Format(time.RFC3339),
		EndTime:         endTime.Format(time.RFC3339),
		EnvironmentID:   environmentID,
		ComponentName:   params.Component,
		ProjectName:     params.Project,
		NamespaceName:   params.Namespace,
		EnvironmentName: params.Environment,
		Limit:           int64(params.Tail),
		SortOrder:       sortOrder,
		LogType:         "runtime",
	}

	// Create observer client and fetch logs
	obsClient := client.NewObserverClient(observerURL, token)
	logResponse, err := obsClient.FetchComponentLogs(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("observer query failed for component %s project=%s, namespace=%s in environment %s at %s: %w",
			params.Component, params.Project, params.Namespace, params.Environment, observerURL, err)
	}

	return logResponse.Logs, nil
}

// findRootEnvironment finds the lowest environment in a deployment pipeline.
// The root environment is the source environment that never appears as a target,
// representing the initial environment where components are first deployed.
func findRootEnvironment(pipeline *gen.DeploymentPipeline) (string, error) {
	if pipeline.Spec == nil || pipeline.Spec.PromotionPaths == nil || len(*pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline %s has no promotion paths defined", pipeline.Metadata.Name)
	}

	// Build a set of all target environments
	targets := make(map[string]bool)
	for _, path := range *pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find source environment that's never a target (the root)
	var rootEnv string
	for _, path := range *pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef.Name == "" {
			continue
		}
		if !targets[path.SourceEnvironmentRef.Name] {
			rootEnv = path.SourceEnvironmentRef.Name
			break
		}
	}

	if rootEnv == "" {
		return "", fmt.Errorf("deployment pipeline %s has no root environment (all sources are also targets)", pipeline.Metadata.Name)
	}

	return rootEnv, nil
}

// resolveObserverURL resolves the observer URL by traversing:
// Environment → DataPlane/ClusterDataPlane → ObservabilityPlane/ClusterObservabilityPlane → observerURL
// This mirrors the server-side GetEnvironmentObserverURL implementation.
func resolveObserverURL(ctx context.Context, apiClient client.Interface, namespace, envName string) (string, error) {
	env, err := apiClient.GetEnvironment(ctx, namespace, envName)
	if err != nil {
		return "", fmt.Errorf("failed to get environment %s: %w", envName, err)
	}

	if env.Spec == nil || env.Spec.DataPlaneRef == nil {
		return "", fmt.Errorf("environment %s has no data plane reference", envName)
	}

	ref := env.Spec.DataPlaneRef

	switch ref.Kind {
	case gen.EnvironmentSpecDataPlaneRefKindClusterDataPlane:
		return resolveObserverURLFromClusterDataPlane(ctx, apiClient, ref.Name)

	case gen.EnvironmentSpecDataPlaneRefKindDataPlane:
		return resolveObserverURLFromDataPlane(ctx, apiClient, namespace, ref.Name)

	default:
		return "", fmt.Errorf("unsupported dataPlaneRef kind %q for environment %s", ref.Kind, envName)
	}
}

// resolveObserverURLFromDataPlane resolves the observer URL from a namespaced DataPlane.
// If the DataPlane has an observabilityPlaneRef, it follows it (supports both ObservabilityPlane
// and ClusterObservabilityPlane kinds). If nil, defaults to ObservabilityPlane named "default".
func resolveObserverURLFromDataPlane(ctx context.Context, apiClient client.Interface, namespace, dpName string) (string, error) {
	dp, err := apiClient.GetDataPlane(ctx, namespace, dpName)
	if err != nil {
		return "", fmt.Errorf("failed to get data plane %s: %w", dpName, err)
	}

	// If no observabilityPlaneRef, default to ObservabilityPlane named "default"
	if dp.Spec == nil || dp.Spec.ObservabilityPlaneRef == nil {
		return getObserverURLFromObservabilityPlane(ctx, apiClient, namespace, defaultPlaneName)
	}

	obsRef := dp.Spec.ObservabilityPlaneRef
	switch obsRef.Kind {
	case gen.ObservabilityPlaneRefKindObservabilityPlane:
		return getObserverURLFromObservabilityPlane(ctx, apiClient, namespace, obsRef.Name)
	case gen.ObservabilityPlaneRefKindClusterObservabilityPlane:
		return getObserverURLFromClusterObservabilityPlane(ctx, apiClient, obsRef.Name)
	default:
		return "", fmt.Errorf("unsupported observabilityPlaneRef kind %q", obsRef.Kind)
	}
}

// resolveObserverURLFromClusterDataPlane resolves the observer URL from a ClusterDataPlane.
// If the ClusterDataPlane has an observabilityPlaneRef, it follows it.
// If nil, defaults to ClusterObservabilityPlane named "default".
func resolveObserverURLFromClusterDataPlane(ctx context.Context, apiClient client.Interface, cdpName string) (string, error) {
	cdp, err := apiClient.GetClusterDataPlane(ctx, cdpName)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster data plane %s: %w", cdpName, err)
	}

	planeName := defaultPlaneName
	if cdp.Spec != nil && cdp.Spec.ObservabilityPlaneRef != nil {
		planeName = cdp.Spec.ObservabilityPlaneRef.Name
	}

	return getObserverURLFromClusterObservabilityPlane(ctx, apiClient, planeName)
}

// getObserverURLFromObservabilityPlane fetches a namespaced ObservabilityPlane and returns its observer URL.
func getObserverURLFromObservabilityPlane(ctx context.Context, apiClient client.Interface, namespace, name string) (string, error) {
	op, err := apiClient.GetObservabilityPlane(ctx, namespace, name)
	if err != nil {
		return "", fmt.Errorf("failed to get observability plane %s: %w", name, err)
	}
	if op.Spec == nil || op.Spec.ObserverURL == nil || *op.Spec.ObserverURL == "" {
		return "", fmt.Errorf("observer URL not configured in observability plane %s", name)
	}
	return *op.Spec.ObserverURL, nil
}

// getObserverURLFromClusterObservabilityPlane fetches a ClusterObservabilityPlane and returns its observer URL.
func getObserverURLFromClusterObservabilityPlane(ctx context.Context, apiClient client.Interface, name string) (string, error) {
	cop, err := apiClient.GetClusterObservabilityPlane(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster observability plane %s: %w", name, err)
	}
	if cop.Spec == nil || cop.Spec.ObserverURL == nil || *cop.Spec.ObserverURL == "" {
		return "", fmt.Errorf("observer URL not configured in cluster observability plane %s", name)
	}
	return *cop.Spec.ObserverURL, nil
}
