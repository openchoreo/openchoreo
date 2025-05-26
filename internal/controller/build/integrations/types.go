// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package integrations

import (
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

type BuildContext struct {
	Component       *choreov1.Component
	DeploymentTrack *choreov1.DeploymentTrack
	Build           *choreov1.Build
}
