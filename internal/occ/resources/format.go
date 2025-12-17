// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Common error messages used across resource implementations
const (
	ErrFormatUnsupported = "unsupported output format: %s"
)

// FormatStatusWithReason formats status with a reason in parentheses
func FormatStatusWithReason(status, reason string) string {
	return fmt.Sprintf("%s (%s)", status, reason)
}

// FormatStatusWithMessage formats status with reason and message
func FormatStatusWithMessage(status, reason, message string) string {
	return fmt.Sprintf("%s: %s - %s", status, reason, message)
}

// FormatStatusWithType formats status with type and reason
func FormatStatusWithType(typeName, reason string) string {
	return fmt.Sprintf("%s: %s", typeName, reason)
}

// FormatAge returns a human-readable string representing the time since the given time
func FormatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	duration := time.Since(t)
	return FormatDurationShort(duration)
}

// FormatDurationShort formats a duration as a short, human-readable string
// Suitable for age display in tables
func FormatDurationShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		m := d / time.Minute
		return fmt.Sprintf("%dm", m)
	} else if d < 24*time.Hour {
		h := d / time.Hour
		return fmt.Sprintf("%dh", h)
	} else {
		d := d / (24 * time.Hour)
		return fmt.Sprintf("%dd", d)
	}
}

// FormatDuration formats a time.Duration into a readable string
// More detailed than FormatDurationShort, suitable for build durations, etc.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		m := d / time.Minute
		s := (d % time.Minute) / time.Second
		return fmt.Sprintf("%dm%ds", m, s)
	} else {
		h := d / time.Hour
		m := (d % time.Hour) / time.Minute
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

// FormatNameWithDisplayName returns a formatted name with display name in parentheses if different
func FormatNameWithDisplayName(name, displayName string) string {
	if displayName != "" && displayName != name {
		return fmt.Sprintf("%s (%s)", name, displayName)
	}
	return name
}

// FormatBoolAsYesNo formats a boolean as "Yes" or "No"
func FormatBoolAsYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// GetPlaceholder returns a placeholder string (typically "-") for empty values
func GetPlaceholder() string {
	return "-"
}

// FormatValueOrPlaceholder returns the value or a placeholder if empty
func FormatValueOrPlaceholder(value string) string {
	if value == "" {
		return GetPlaceholder()
	}
	return value
}

// GetStatus extracts the latest status from conditions.
func GetStatus(conditions []metav1.Condition, defaultStatus string) string {
	if len(conditions) == 0 {
		return defaultStatus
	}
	latest := conditions[0]
	for _, c := range conditions[1:] {
		if c.LastTransitionTime.After(latest.LastTransitionTime.Time) {
			latest = c
		}
	}
	return string(latest.Status)
}

// PrintTable prints slices of rows in tabular format.
func PrintTable(headers []string, rows [][]string) error {
	if len(rows) == 0 {
		fmt.Println("No resources found.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

// GetK8sObjectYAMLFromCRDWithLabels retrieves a K8s object matching the given parameters
// and returns it as YAML with runtime fields cleaned.
func GetK8sObjectYAMLFromCRDWithLabels(group, version, kind, namespace string, labels map[string]string) (string, error) {
	k8sClient, err := GetClient()
	if err != nil {
		return "", err
	}

	gvk := schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    kind + "List",
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	if err := k8sClient.List(context.Background(), list,
		client.InNamespace(namespace),
		client.MatchingLabels(labels)); err != nil {
		return "", fmt.Errorf("failed to list objects: %w", err)
	}

	if len(list.Items) == 0 {
		return "", fmt.Errorf("no object found with labels %v", labels)
	}
	if len(list.Items) > 1 {
		return "", fmt.Errorf("multiple objects found with labels %v", labels)
	}

	obj := &list.Items[0]

	obj.SetManagedFields(nil)
	obj.SetGeneration(0)
	obj.SetResourceVersion("")
	obj.SetUID("")

	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object to YAML %w", err)
	}

	return string(yamlBytes), nil
}
