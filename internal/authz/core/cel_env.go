// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sync"

	"github.com/google/cel-go/cel"
)

const conditionTypeResource = "resource"

// KnownCELRoots is the set of CEL root variable names declared in the shared env.
// Add new roots here when extending the ABAC attribute model.
var KnownCELRoots = map[string]bool{
	conditionTypeResource: true,
}

var (
	sharedCELEnv     *cel.Env
	sharedCELEnvOnce sync.Once
	sharedCELEnvErr  error
)

// GetCELEnv returns the shared CEL environment used by both the runtime evaluator
// and the admission webhook, so validation matches evaluation exactly.
func GetCELEnv() (*cel.Env, error) {
	sharedCELEnvOnce.Do(func() {
		sharedCELEnv, sharedCELEnvErr = cel.NewEnv(
			cel.Variable(conditionTypeResource, cel.MapType(cel.StringType, cel.DynType)),
		)
	})
	return sharedCELEnv, sharedCELEnvErr
}
