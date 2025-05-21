/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package integrations

import (
	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

type BuildContext struct {
	Component       *choreov1.Component
	DeploymentTrack *choreov1.DeploymentTrack
	Build           *choreov1.Build
}
