// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"testing"
)

func TestPipelineCleanupFinalizerValue(t *testing.T) {
	const want = "openchoreo.dev/deployment-pipeline-cleanup"
	if PipelineCleanupFinalizer != want {
		t.Errorf("PipelineCleanupFinalizer: got %q, want %q", PipelineCleanupFinalizer, want)
	}
}
