// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import "errors"

var (
	ErrDeploymentPipelineNotFound      = errors.New("deployment pipeline not found")
	ErrDeploymentPipelineAlreadyExists = errors.New("deployment pipeline already exists")
)
