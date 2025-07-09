// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package integrations

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

type BuildContext struct {
	Registry        openchoreov1alpha1.Registry
	Component       *openchoreov1alpha1.Component
	DeploymentTrack *openchoreov1alpha1.DeploymentTrack
	Build           *openchoreov1alpha1.Build
}
