// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package pipeline

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PipelineInfo holds parsed deployment pipeline information
type PipelineInfo struct {
	Name            string
	RootEnvironment string
	Environments    []string            // All environments in order
	PromotionPaths  map[string][]string // source -> targets
	EnvPosition     map[string]int      // env -> position in pipeline
}

// PromotionPath represents a single promotion path from source to targets
type PromotionPath struct {
	SourceEnvironmentRef  string
	TargetEnvironmentRefs []string
}

// ParsePipeline extracts pipeline information from unstructured resource
func ParsePipeline(pipeline *unstructured.Unstructured) (*PipelineInfo, error) {
	if pipeline == nil {
		return nil, fmt.Errorf("pipeline is nil")
	}

	name := pipeline.GetName()

	// Parse promotion paths from spec.promotionPaths
	promotionPathsRaw, found, err := unstructured.NestedSlice(pipeline.Object, "spec", "promotionPaths")
	if err != nil {
		return nil, fmt.Errorf("failed to get spec.promotionPaths: %w", err)
	}
	if !found || len(promotionPathsRaw) == 0 {
		return nil, fmt.Errorf("deployment pipeline %s has no promotion paths defined", name)
	}

	// Build promotion paths map and collect all environments
	promotionPaths := make(map[string][]string)
	allEnvs := make(map[string]bool)

	for _, pathRaw := range promotionPathsRaw {
		pathMap, ok := pathRaw.(map[string]interface{})
		if !ok {
			continue
		}

		source, _ := pathMap["sourceEnvironmentRef"].(string)
		if source == "" {
			continue
		}
		allEnvs[source] = true

		targetsRaw, ok := pathMap["targetEnvironmentRefs"].([]interface{})
		if !ok {
			continue
		}

		var targets []string
		for _, targetRaw := range targetsRaw {
			targetMap, ok := targetRaw.(map[string]interface{})
			if !ok {
				continue
			}
			targetName, _ := targetMap["name"].(string)
			if targetName != "" {
				targets = append(targets, targetName)
				allEnvs[targetName] = true
			}
		}

		promotionPaths[source] = targets
	}

	// Find root environment
	rootEnv, err := FindRootEnvironment(pipeline)
	if err != nil {
		return nil, err
	}

	// Build environment position map using BFS from root
	envPosition := make(map[string]int)
	envPosition[rootEnv] = 0

	// BFS to assign positions
	queue := []string{rootEnv}
	visited := make(map[string]bool)
	visited[rootEnv] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		targets := promotionPaths[current]
		for _, target := range targets {
			if !visited[target] {
				visited[target] = true
				envPosition[target] = envPosition[current] + 1
				queue = append(queue, target)
			}
		}
	}

	// Build ordered list of environments
	environments := make([]string, 0, len(allEnvs))
	for env := range allEnvs {
		environments = append(environments, env)
	}

	return &PipelineInfo{
		Name:            name,
		RootEnvironment: rootEnv,
		Environments:    environments,
		PromotionPaths:  promotionPaths,
		EnvPosition:     envPosition,
	}, nil
}

// FindRootEnvironment finds the root environment (never appears as a target)
// This mirrors the logic in internal/controller/component/controller.go:findRootEnvironment
func FindRootEnvironment(pipeline *unstructured.Unstructured) (string, error) {
	if pipeline == nil {
		return "", fmt.Errorf("pipeline is nil")
	}

	name := pipeline.GetName()

	// Parse promotion paths
	promotionPathsRaw, found, err := unstructured.NestedSlice(pipeline.Object, "spec", "promotionPaths")
	if err != nil {
		return "", fmt.Errorf("failed to get spec.promotionPaths: %w", err)
	}
	if !found || len(promotionPathsRaw) == 0 {
		return "", fmt.Errorf("deployment pipeline %s has no promotion paths defined", name)
	}

	// Build a set of all target environments
	targets := make(map[string]bool)
	for _, pathRaw := range promotionPathsRaw {
		pathMap, ok := pathRaw.(map[string]interface{})
		if !ok {
			continue
		}

		targetsRaw, ok := pathMap["targetEnvironmentRefs"].([]interface{})
		if !ok {
			continue
		}

		for _, targetRaw := range targetsRaw {
			targetMap, ok := targetRaw.(map[string]interface{})
			if !ok {
				continue
			}
			targetName, _ := targetMap["name"].(string)
			if targetName != "" {
				targets[targetName] = true
			}
		}
	}

	// Find source environment that's never a target (the root)
	var rootEnv string
	for _, pathRaw := range promotionPathsRaw {
		pathMap, ok := pathRaw.(map[string]interface{})
		if !ok {
			continue
		}

		source, _ := pathMap["sourceEnvironmentRef"].(string)
		if source == "" {
			continue
		}

		if !targets[source] {
			rootEnv = source
			break
		}
	}

	if rootEnv == "" {
		return "", fmt.Errorf("deployment pipeline %s has no root environment (all sources are also targets)", name)
	}

	return rootEnv, nil
}

// ValidateEnvironment checks if an environment exists in the pipeline
func (p *PipelineInfo) ValidateEnvironment(envName string) error {
	if envName == "" {
		return fmt.Errorf("environment name is empty")
	}

	_, exists := p.EnvPosition[envName]
	if !exists {
		return fmt.Errorf("environment %q does not exist in deployment pipeline %q", envName, p.Name)
	}

	return nil
}

// IsRootEnvironment returns true if the given environment is the root
func (p *PipelineInfo) IsRootEnvironment(envName string) bool {
	return envName == p.RootEnvironment
}

// GetPreviousEnvironment returns the source environment for the given target
// Returns empty string if this is the root environment
func (p *PipelineInfo) GetPreviousEnvironment(envName string) (string, error) {
	if envName == "" {
		return "", fmt.Errorf("environment name is empty")
	}

	// Check if environment exists
	if err := p.ValidateEnvironment(envName); err != nil {
		return "", err
	}

	// If it's the root environment, there's no previous environment
	if p.IsRootEnvironment(envName) {
		return "", nil
	}

	// Find the source environment that has this environment as a target
	for source, targets := range p.PromotionPaths {
		for _, target := range targets {
			if target == envName {
				return source, nil
			}
		}
	}

	return "", fmt.Errorf("no previous environment found for %q in pipeline %q", envName, p.Name)
}

// GetEnvironmentPosition returns the position of the environment in the promotion order
// 0 = root environment, higher numbers = later in promotion chain
func (p *PipelineInfo) GetEnvironmentPosition(envName string) (int, error) {
	if envName == "" {
		return -1, fmt.Errorf("environment name is empty")
	}

	position, exists := p.EnvPosition[envName]
	if !exists {
		return -1, fmt.Errorf("environment %q does not exist in deployment pipeline %q", envName, p.Name)
	}

	return position, nil
}