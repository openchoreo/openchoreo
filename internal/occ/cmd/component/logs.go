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
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// ComponentLogs fetches and displays logs for a component
func (c *CompImpl) ComponentLogs(params api.ComponentLogsParams) error {
	ctx := context.Background()

	// Create API client
	apiClient, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Get component to resolve UID
	component, err := apiClient.GetComponent(ctx, params.Namespace, params.Component)
	if err != nil {
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

	// Get observer URL for the environment
	observerURL, err := apiClient.GetEnvironmentObserverURL(ctx, params.Namespace, envName)
	if err != nil {
		return fmt.Errorf("failed to get observer URL: %w", err)
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

	componentUID := ""
	if component.Metadata.Uid != nil {
		componentUID = *component.Metadata.Uid
	}

	environmentUID := ""
	if environment.Metadata.Uid != nil {
		environmentUID = *environment.Metadata.Uid
	}

	if params.Follow {
		return c.followLogs(ctx, observerURL, credential.Token, componentUID, environmentUID, params, startTime, endTime)
	}

	return c.fetchAndPrintLogs(ctx, observerURL, credential.Token, componentUID, environmentUID, params, startTime, endTime)
}

// fetchAndPrintLogs fetches logs for a given time range and prints them
func (c *CompImpl) fetchAndPrintLogs(
	ctx context.Context,
	observerURL string,
	token string,
	componentID string,
	environmentID string,
	params api.ComponentLogsParams,
	startTime time.Time,
	endTime time.Time,
) error {
	logs, err := c.fetchLogs(ctx, observerURL, token, componentID, environmentID, params, startTime, endTime)
	if err != nil {
		return err
	}

	for _, log := range logs {
		fmt.Printf("%s %s\n", log.Timestamp, log.Log)
	}

	return nil
}

// followLogs continuously fetches and prints new logs
func (c *CompImpl) followLogs(
	ctx context.Context,
	observerURL string,
	token string,
	componentID string,
	environmentID string,
	params api.ComponentLogsParams,
	startTime time.Time,
	endTime time.Time,
) error {
	// Set up signal handling for graceful shutdown with cancelable context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initial fetch
	logs, err := c.fetchLogs(ctx, observerURL, token, componentID, environmentID, params, startTime, endTime)
	if err != nil {
		return err
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

			logs, err := c.fetchLogs(ctx, observerURL, token, componentID, environmentID, params, startTime, endTime)
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
func (c *CompImpl) fetchLogs(
	ctx context.Context,
	observerURL string,
	token string,
	componentID string,
	environmentID string,
	params api.ComponentLogsParams,
	startTime time.Time,
	endTime time.Time,
) ([]client.LogEntry, error) {
	reqBody := client.ComponentLogsRequest{
		StartTime:       startTime.Format(time.RFC3339),
		EndTime:         endTime.Format(time.RFC3339),
		EnvironmentID:   environmentID,
		ComponentName:   params.Component,
		ProjectName:     params.Project,
		NamespaceName:   params.Namespace,
		EnvironmentName: params.Environment,
		SortOrder:       "asc",
		LogType:         "runtime",
	}

	// Create observer client and fetch logs
	obsClient := client.NewObserverClient(observerURL, token)
	logResponse, err := obsClient.FetchComponentLogs(ctx, componentID, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for component %s/%s/%s in environment %s from observer %s: %w",
			params.Namespace, params.Project, params.Component, params.Environment, observerURL, err)
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
		if path.SourceEnvironmentRef == "" {
			continue
		}
		if !targets[path.SourceEnvironmentRef] {
			rootEnv = path.SourceEnvironmentRef
			break
		}
	}

	if rootEnv == "" {
		return "", fmt.Errorf("deployment pipeline %s has no root environment (all sources are also targets)", pipeline.Metadata.Name)
	}

	return rootEnv, nil
}
