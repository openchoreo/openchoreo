// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package integrations

import (
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

type BuildContext struct {
	Registry        choreov1.Registry
	Component       *choreov1.Component
	DeploymentTrack *choreov1.DeploymentTrack
	Build           *choreov1.Build
}
