// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/interpreter"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// celEnv is the shared CEL environment for evaluating authz conditions.
var (
	celEnv     *cel.Env
	celEnvOnce sync.Once
	celEnvErr  error
)

const conditionTypeResource = "resource"

func getCELEnv() (*cel.Env, error) {
	celEnvOnce.Do(func() {
		celEnv, celEnvErr = cel.NewEnv(
			cel.Variable(conditionTypeResource, cel.MapType(cel.StringType, cel.DynType)),
		)
	})
	return celEnv, celEnvErr
}

// compileCEL compiles a CEL expression and returns a ready-to-evaluate Program.
func compileCEL(expr string) (cel.Program, error) {
	env, err := getCELEnv()
	if err != nil {
		return nil, fmt.Errorf("CEL environment unavailable: %w", err)
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile error: %w", issues.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program construction error: %w", err)
	}
	return prg, nil
}

// buildCelActivation constructs the CEL activation map from the request context,
// gated by the allowed attributes for the action.
// Every allowed attribute is bound — either to the value from ctx
// or to a type-appropriate zero — so CEL expressions never fault on unbound variables
// when a request omits an optional field.
func buildCelActivation(authzCtx authzcore.Context, allowedAttrs []authzcore.AttributeSpec) (interpreter.Activation, error) {
	if len(allowedAttrs) == 0 {
		return interpreter.NewActivation(map[string]any{})
	}

	ctxAttrs, err := convertCtxToAttrMap(authzCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to convert context for CEL activation: %w", err)
	}

	activationByRoot := map[string]map[string]any{}
	for _, spec := range allowedAttrs {
		root, leaf := spec.Root(), spec.Leaf()
		if root == "" || leaf == "" {
			continue
		}
		if activationByRoot[root] == nil {
			activationByRoot[root] = map[string]any{}
		}
		attrValue, ok := ctxAttrs[root][leaf]
		if !ok {
			attrValue = zeroForCELType(spec.CELType)
		}
		activationByRoot[root][leaf] = attrValue
	}

	activation := make(map[string]any, len(activationByRoot))
	for root, leafValues := range activationByRoot {
		activation[root] = leafValues
	}
	return interpreter.NewActivation(activation)
}

// convertCtxToAttrMap JSON-round-trips ctx into a two-level map (root → leaf → value)
// so the json tags on authzcore.Context drive the CEL variable names automatically.
func convertCtxToAttrMap(ctx authzcore.Context) (map[string]map[string]any, error) {
	ctxJSON, err := json.Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize context for CEL activation: %w", err)
	}
	var ctxAttrs map[string]map[string]any
	if err := json.Unmarshal(ctxJSON, &ctxAttrs); err != nil {
		return nil, fmt.Errorf("failed to deserialize context for CEL activation: %w", err)
	}
	return ctxAttrs, nil
}

// zeroForCELType returns the Go zero value corresponding to a CEL type
func zeroForCELType(t *cel.Type) any {
	switch t {
	case cel.StringType:
		return ""
	case cel.BoolType:
		return false
	case cel.IntType:
		return int64(0)
	case cel.DoubleType:
		return 0.0
	default:
		return nil
	}
}
