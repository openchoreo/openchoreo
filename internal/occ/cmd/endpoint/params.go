// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import "time"

// ListParams defines parameters for listing endpoints
type ListParams struct {
	Namespace string
	Project   string
}

// GetParams defines parameters for getting a single endpoint
type GetParams struct {
	Namespace    string
	EndpointName string
}

// DeleteParams defines parameters for deleting a single endpoint
type DeleteParams struct {
	Namespace    string
	EndpointName string
}

// PromoteParams defines parameters for promoting an endpoint to a target environment
type PromoteParams struct {
	Namespace    string
	EndpointName string
	Environment  string
}

// RolloutParams defines the parameters every rollout command takes
type RolloutParams struct {
	Namespace    string
	EndpointName string
	DryRun       bool
	Output       string
}

// RolloutStartParams defines parameters for starting a rollout
type RolloutStartParams struct {
	RolloutParams
	File string
	Plan RolloutPlanFlags
}

// RolloutStepParams defines parameters for stepping or aborting a rollout in an environment
type RolloutStepParams struct {
	RolloutParams
	Environment  string
	To           *int
	ExpectedStep *int
	Back         bool
	Last         bool
}

// RolloutFinishParams defines parameters for finishing a rollout
type RolloutFinishParams struct {
	RolloutParams
	Keep string
}

// RolloutGetParams defines parameters for showing a rollout
type RolloutGetParams struct {
	RolloutParams
	Environment string
	Watch       bool
	Interval    time.Duration
}

// RolloutPlanFlags builds a rollout plan from flags instead of a file
type RolloutPlanFlags struct {
	Strategy   string
	Stable     string
	Candidates []string
	Steps      []int32
	Header     string
	Cookie     string
}

// RolloutWaitParams defines parameters for waiting on an environment
type RolloutWaitParams struct {
	RolloutParams
	Environment string
	Timeout     time.Duration
	Interval    time.Duration
}

// RolloutSetParams defines parameters for hand-set weights in an environment
type RolloutSetParams struct {
	RolloutParams
	Environment string
	Weights     map[string]int32
}
