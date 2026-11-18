// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
)

// matchProblems reports the backend matches the data planes can't express
// the same way, before a release is cut.
func matchProblems(ep *openchoreov1alpha1.Endpoint) []backendProblem {
	msgs := endpointpipeline.MatchProblems(ep.Spec.BackendRefs)
	problems := make([]backendProblem, 0, len(msgs))
	for _, msg := range msgs {
		problems = append(problems, backendProblem{ReasonInvalidMatch, msg})
	}
	return problems
}
