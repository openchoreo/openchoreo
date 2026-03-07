// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/controller"
)

func TestPipelineCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/deployment-pipeline-cleanup"
	if PipelineCleanupFinalizer != want {
		t.Errorf("PipelineCleanupFinalizer: got %q, want %q", PipelineCleanupFinalizer, want)
	}
}

func TestReconcilerUsesTypeAvailable(t *testing.T) {
	// The DeploymentPipeline controller uses controller.TypeAvailable for its status condition.
	// Verify the constant value hasn't drifted.
	const want = "Available"
	if controller.TypeAvailable != want {
		t.Errorf("controller.TypeAvailable: got %q, want %q", controller.TypeAvailable, want)
	}
}
