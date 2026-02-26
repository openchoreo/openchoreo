// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/controller"
)

// extractCommonMeta returns the common metadata fields from a Kubernetes object
// that are useful for AI agents: name, namespace, displayName, description, createdAt.
func extractCommonMeta(obj client.Object) map[string]any {
	m := map[string]any{
		"name": obj.GetName(),
	}
	if ns := obj.GetNamespace(); ns != "" {
		m["namespace"] = ns
	}
	if dn := controller.GetDisplayName(obj); dn != "" {
		m["displayName"] = dn
	}
	if desc := controller.GetDescription(obj); desc != "" {
		m["description"] = desc
	}
	if ts := obj.GetCreationTimestamp(); !ts.IsZero() {
		m["createdAt"] = ts.UTC().Format("2006-01-02T15:04:05Z")
	}
	return m
}

// readyStatus extracts a compact status string from a slice of conditions.
// It looks for the "Ready" condition and returns "Ready" if True, the Reason
// if not True, or an empty string if no Ready condition exists.
func readyStatus(conditions []metav1.Condition) string {
	for i := range conditions {
		if conditions[i].Type == "Ready" {
			if conditions[i].Status == metav1.ConditionTrue {
				return "Ready"
			}
			return conditions[i].Reason
		}
	}
	return ""
}

// conditionsSummary returns a compact representation of conditions for detail
// views, stripping lastTransitionTime and observedGeneration.
func conditionsSummary(conditions []metav1.Condition) []map[string]any {
	if len(conditions) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(conditions))
	for i := range conditions {
		c := map[string]any{
			"type":   conditions[i].Type,
			"status": string(conditions[i].Status),
		}
		if conditions[i].Reason != "" {
			c["reason"] = conditions[i].Reason
		}
		if conditions[i].Message != "" {
			c["message"] = conditions[i].Message
		}
		result = append(result, c)
	}
	return result
}

// transformList maps a slice of items through a transform function.
func transformList[T any](items []T, fn func(T) map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for i := range items {
		result = append(result, fn(items[i]))
	}
	return result
}

// wrapTransformedList combines transformList with wrapList, transforming the
// items and wrapping them in a keyed map suitable for MCP structured content.
func wrapTransformedList[T any](key string, items []T, nextCursor string, fn func(T) map[string]any) map[string]any {
	return wrapList(key, transformList(items, fn), nextCursor)
}

// mutationResult returns a compact confirmation for mutating operations.
// It includes name, namespace (if set), and the action performed, plus any
// extra key-value pairs passed via the extras map.
func mutationResult(obj client.Object, action string, extras ...map[string]any) map[string]any {
	m := map[string]any{
		"name":   obj.GetName(),
		"action": action,
	}
	if ns := obj.GetNamespace(); ns != "" {
		m["namespace"] = ns
	}
	for _, extra := range extras {
		maps.Copy(m, extra)
	}
	return m
}

// setIfNotEmpty sets a key in the map only if the string value is non-empty.
func setIfNotEmpty(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
