// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// FindRootEnvironment finds the root environment in a deployment pipeline: the
// source environment that never appears as a target, where components and
// endpoints are first deployed. See DeploymentPipeline.RootEnvironment, which
// the rollback guard also uses, so both agree on which environment auto-deploy
// targets.
func FindRootEnvironment(pipeline *openchoreov1alpha1.DeploymentPipeline) (string, error) {
	return pipeline.RootEnvironment()
}
