// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// FindLowestEnvironment finds the environment that is not a target in any promotion path.
func FindLowestEnvironment(pipeline *gen.DeploymentPipeline) (string, error) {
	if pipeline.Spec == nil || pipeline.Spec.PromotionPaths == nil || len(*pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	targets := make(map[string]bool)
	for _, path := range *pipeline.Spec.PromotionPaths {
		for _, targetRef := range path.TargetEnvironmentRefs {
			targets[targetRef.Name] = true
		}
	}

	for _, path := range *pipeline.Spec.PromotionPaths {
		if !targets[path.SourceEnvironmentRef.Name] {
			return path.SourceEnvironmentRef.Name, nil
		}
	}

	// Fallback: return the first source
	return (*pipeline.Spec.PromotionPaths)[0].SourceEnvironmentRef.Name, nil
}

// FindSourceEnvironment finds the source environment for a given target environment in the pipeline.
func FindSourceEnvironment(pipeline *gen.DeploymentPipeline, targetEnv string) (string, error) {
	if pipeline.Spec == nil || pipeline.Spec.PromotionPaths == nil || len(*pipeline.Spec.PromotionPaths) == 0 {
		return "", fmt.Errorf("deployment pipeline has no promotion paths")
	}

	// Search through promotion paths to find source for target
	for _, path := range *pipeline.Spec.PromotionPaths {
		for _, targetRef := range path.TargetEnvironmentRefs {
			if targetRef.Name == targetEnv {
				return path.SourceEnvironmentRef.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no promotion path found for target environment '%s'", targetEnv)
}
