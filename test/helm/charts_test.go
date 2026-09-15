// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package helm holds the tests that render the shipped Helm charts and assert on
// what an install actually gets. They exercise no Go code of their own, so they
// live here rather than under a cmd/ package whose binary they have nothing to
// do with.
package helm

// The charts under test, one set of paths for every file in this package. They
// are relative to this directory, which sits at the same depth as cmd/, so a
// chart that moves is fixed here once.
const (
	chartsDir = "../../install/helm"

	controlPlaneChart       = chartsDir + "/openchoreo-control-plane"
	dataPlaneChart          = chartsDir + "/openchoreo-data-plane"
	workflowPlaneChart      = chartsDir + "/openchoreo-workflow-plane"
	observabilityPlaneChart = chartsDir + "/openchoreo-observability-plane"
)
