/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package integrations

type BuildWorkflowStep string

const (
	CloneStep BuildWorkflowStep = "clone-step"
	BuildStep BuildWorkflowStep = "build-step"
	PushStep  BuildWorkflowStep = "push-step"
)

type StepPhase string

// Workflow and node statuses
const (
	Running   StepPhase = "Running"
	Succeeded StepPhase = "Succeeded"
	Failed    StepPhase = "Failed"
)
