// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// ResolveEmbeddedTraitBindings resolves CEL expressions in an embedded trait's parameter
// and environmentConfigs bindings against the component context, producing concrete maps
// suitable for passing into BuildTraitContext as ResolvedParameters / ResolvedEnvironmentConfigs.
//
// Values in the embedded trait bindings can be:
//   - Concrete values (locked by PE): passed through as-is
//   - CEL expressions like "${parameters.storage.mountPath}": evaluated against the component context
//
// Counterpart to ExtractTraitInstanceBindings, which performs the equivalent step for
// component-level traits (where no CEL evaluation is needed).
func ResolveEmbeddedTraitBindings(
	engine *template.Engine,
	embeddedTrait v1alpha1.ComponentTypeTrait,
	componentContextMap map[string]any,
) (resolvedParams map[string]any, resolvedEnvConfigs map[string]any, err error) {
	resolvedParams, err = resolveBindings(engine, embeddedTrait.Parameters, componentContextMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve embedded trait %s/%s parameters: %w",
			embeddedTrait.Name, embeddedTrait.InstanceName, err)
	}

	resolvedEnvConfigs, err = resolveBindings(engine, embeddedTrait.EnvironmentConfigs, componentContextMap)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve embedded trait %s/%s environmentConfigs: %w",
			embeddedTrait.Name, embeddedTrait.InstanceName, err)
	}

	return resolvedParams, resolvedEnvConfigs, nil
}

// resolveBindings takes a RawExtension containing mixed concrete values and CEL expressions,
// evaluates all CEL expressions against the given context, and returns a map with all values
// resolved to concrete types.
func resolveBindings(
	engine *template.Engine,
	raw *runtime.RawExtension,
	contextMap map[string]any,
) (map[string]any, error) {
	if raw == nil || raw.Raw == nil {
		return nil, nil
	}

	var data map[string]any
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bindings: %w", err)
	}

	resolved, err := engine.Render(data, contextMap)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate CEL bindings: %w", err)
	}

	resolvedMap, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resolved bindings is not a map, got type %T", resolved)
	}

	return resolvedMap, nil
}
