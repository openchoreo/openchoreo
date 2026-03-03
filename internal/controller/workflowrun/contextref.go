// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// resolveContextRefs resolves each contextRef declared in the Workflow spec.
// For each ref whose name evaluates to a non-empty string, it fetches the
// referenced CR from the cluster and returns its spec keyed by the ref's id.
// Refs whose name evaluates to empty are silently skipped.
func (r *Reconciler) resolveContextRefs(
	ctx context.Context,
	contextRefs []openchoreodevv1alpha1.ContextRef,
	celContext map[string]any,
	namespace string,
) (map[string]any, error) {
	if len(contextRefs) == 0 {
		return nil, nil
	}

	engine := template.NewEngine()
	resolved := make(map[string]any, len(contextRefs))

	for _, ref := range contextRefs {
		// Evaluate the name field which may contain CEL expressions
		name, err := evaluateContextRefName(engine, ref.Name, celContext)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate name for contextRef %q: %w", ref.ID, err)
		}

		// Skip refs with empty names
		if name == "" {
			continue
		}

		specData, err := r.fetchContextRefSpec(ctx, ref, name, namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve contextRef %q: %w", ref.ID, err)
		}

		resolved[ref.ID] = map[string]any{
			"spec": specData,
		}
	}

	return resolved, nil
}

// evaluateContextRefName renders a contextRef name string through the template engine.
// The name may contain CEL expressions like ${parameters.repository.secretRef}.
// Returns the evaluated string, or empty string if the expression evaluates to empty.
func evaluateContextRefName(engine *template.Engine, name string, celContext map[string]any) (string, error) {
	result, err := engine.Render(name, celContext)
	if err != nil {
		return "", err
	}

	str, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("name expression must evaluate to a string, got %T", result)
	}

	return str, nil
}

// fetchContextRefSpec fetches the referenced CR and returns its spec as a map.
func (r *Reconciler) fetchContextRefSpec(
	ctx context.Context,
	ref openchoreodevv1alpha1.ContextRef,
	name string,
	namespace string,
) (map[string]any, error) {
	switch ref.Kind {
	case "SecretReference":
		return r.fetchSecretReferenceSpec(ctx, name, namespace)
	default:
		return nil, fmt.Errorf("unsupported contextRef kind %q, only SecretReference is supported", ref.Kind)
	}
}

// fetchSecretReferenceSpec fetches a SecretReference CR and returns its spec as a map.
func (r *Reconciler) fetchSecretReferenceSpec(ctx context.Context, name, namespace string) (map[string]any, error) {
	secretRef := &openchoreodevv1alpha1.SecretReference{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, secretRef); err != nil {
		return nil, fmt.Errorf("failed to get SecretReference %q in namespace %q: %w", name, namespace, err)
	}

	// Marshal the entire spec to JSON and unmarshal to map for CEL context
	specJSON, err := json.Marshal(secretRef.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SecretReference %q spec: %w", name, err)
	}

	var specMap map[string]any
	if err := json.Unmarshal(specJSON, &specMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SecretReference %q spec: %w", name, err)
	}

	return specMap, nil
}
