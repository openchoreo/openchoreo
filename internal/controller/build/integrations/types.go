// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package integrations

import (
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

type BuildContext struct {
	BuildPlane      *choreov1.BuildPlane
	Component       *choreov1.Component
	DeploymentTrack *choreov1.DeploymentTrack
	Build           *choreov1.Build
}
