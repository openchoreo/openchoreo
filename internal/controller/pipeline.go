// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// FindRootEnvironment finds the root environment in a deployment pipeline.
// The root environment is the source environment that never appears as a target,
// representing the initial environment where components and endpoints are first deployed.
// This is a pure function for easier testing.
func FindRootEnvironment(pipeline *openchoreov1alpha1.DeploymentPipeline) (string, error) {
	if len(pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline %s has no promotion paths defined", pipeline.Name)
	}

	// Build a set of all target environments
	targets := make(map[string]bool)
	for _, path := range pipeline.Spec.PromotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find source environment that's never a target (the root)
	var rootEnv string
	for _, path := range pipeline.Spec.PromotionPaths {
		if path.SourceEnvironmentRef.Name == "" {
			continue
		}
		if !targets[path.SourceEnvironmentRef.Name] {
			rootEnv = path.SourceEnvironmentRef.Name
			break
		}
	}

	if rootEnv == "" {
		return "", fmt.Errorf("deployment pipeline %s has no root environment (all sources are also targets)", pipeline.Name)
	}

	return rootEnv, nil
}
