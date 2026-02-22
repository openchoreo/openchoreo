// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	workflowpipeline "github.com/openchoreo/openchoreo/internal/pipeline/workflow"
)

// parseComponentWorkflowAnnotation parses openchoreo.dev/component-workflow-parameters
// in the format "key1: path1, key2: path2".
func parseComponentWorkflowAnnotation(annotation string) map[string]string {
	result := make(map[string]string)
	if annotation == "" {
		return result
	}

	pairs := strings.Split(annotation, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if key != "" && path != "" {
			result[key] = path
		}
	}

	return result
}

// getNestedStringFromRawExtension returns a nested string value from parameters JSON.
// The leading "parameters." prefix is stripped if present.
// found=false is returned when the path does not exist or parameters are nil.
func getNestedStringFromRawExtension(raw *runtime.RawExtension, dottedPath string) (value string, found bool, err error) {
	if raw == nil || raw.Raw == nil {
		return "", false, nil
	}

	path := strings.TrimSpace(strings.TrimPrefix(dottedPath, "parameters."))
	if path == "" {
		return "", false, fmt.Errorf("path %q is empty", dottedPath)
	}

	var data map[string]any
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return "", false, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	parts := strings.Split(path, ".")
	current := any(data)
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return "", false, fmt.Errorf("path %s: expected object at %s", dottedPath, part)
		}
		next, ok := obj[part]
		if !ok {
			return "", false, nil
		}
		current = next
	}

	str, ok := current.(string)
	if !ok {
		return "", false, fmt.Errorf("path %s: value is not a string", dottedPath)
	}

	return str, true, nil
}

// resolveSecretRefInfo loads SecretReference data and converts it to workflow pipeline context shape.
func (r *Reconciler) resolveSecretRefInfo(ctx context.Context, namespace, secretRefName string) (*workflowpipeline.SecretRefInfo, error) {
	secretRef := &openchoreodevv1alpha1.SecretReference{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      secretRefName,
		Namespace: namespace,
	}, secretRef); err != nil {
		return nil, fmt.Errorf("failed to get SecretReference %q in namespace %q: %w", secretRefName, namespace, err)
	}

	if len(secretRef.Spec.Data) == 0 {
		return nil, fmt.Errorf("SecretReference %q has no data sources", secretRefName)
	}

	dataInfos := make([]workflowpipeline.SecretDataInfo, len(secretRef.Spec.Data))
	for i, dataSource := range secretRef.Spec.Data {
		dataInfos[i] = workflowpipeline.SecretDataInfo{
			SecretKey: dataSource.SecretKey,
			RemoteRef: workflowpipeline.RemoteRefInfo{
				Key:      dataSource.RemoteRef.Key,
				Property: dataSource.RemoteRef.Property,
			},
		}
	}

	secretType := string(secretRef.Spec.Template.Type)
	if secretType == "" {
		secretType = "kubernetes.io/basic-auth"
	}

	return &workflowpipeline.SecretRefInfo{
		Name: secretRefName,
		Type: secretType,
		Data: dataInfos,
	}, nil
}
