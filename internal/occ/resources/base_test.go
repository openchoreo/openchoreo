// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// makeConfigMapWrapper creates a ResourceWrapper wrapping a ConfigMap for testing.
func makeConfigMapWrapper(logicalName, k8sName string) ResourceWrapper[*corev1.ConfigMap] {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: k8sName},
	}
	return ResourceWrapper[*corev1.ConfigMap]{
		Resource:       cm,
		LogicalName:    logicalName,
		KubernetesName: k8sName,
	}
}

// ---- FilterByName ----

func TestFilterByName_EmptyName_ReturnsAll(t *testing.T) {
	items := []ResourceWrapper[*corev1.ConfigMap]{
		makeConfigMapWrapper("a", "k8s-a"),
		makeConfigMapWrapper("b", "k8s-b"),
	}
	got, err := FilterByName(items, "")
	if err != nil {
		t.Fatalf("FilterByName empty name: unexpected error %v", err)
	}
	if len(got) != 2 {
		t.Errorf("FilterByName empty name: got %d items, want 2", len(got))
	}
}

func TestFilterByName_MatchOne(t *testing.T) {
	items := []ResourceWrapper[*corev1.ConfigMap]{
		makeConfigMapWrapper("alpha", "k8s-alpha"),
		makeConfigMapWrapper("beta", "k8s-beta"),
		makeConfigMapWrapper("gamma", "k8s-gamma"),
	}
	got, err := FilterByName(items, "beta")
	if err != nil {
		t.Fatalf("FilterByName match one: unexpected error %v", err)
	}
	if len(got) != 1 || got[0].GetName() != "beta" {
		t.Errorf("FilterByName match one: got %v, want single 'beta' item", got)
	}
}

func TestFilterByName_NoMatch_ReturnsError(t *testing.T) {
	items := []ResourceWrapper[*corev1.ConfigMap]{
		makeConfigMapWrapper("alpha", "k8s-alpha"),
	}
	_, err := FilterByName(items, "nonexistent")
	if err == nil {
		t.Error("FilterByName no match: expected error, got nil")
	}
}

func TestFilterByName_EmptyList_ReturnsError(t *testing.T) {
	var items []ResourceWrapper[*corev1.ConfigMap]
	_, err := FilterByName(items, "anything")
	if err == nil {
		t.Error("FilterByName empty list: expected error for non-empty name, got nil")
	}
}

func TestFilterByName_MultipleMatchingSameName(t *testing.T) {
	items := []ResourceWrapper[*corev1.ConfigMap]{
		makeConfigMapWrapper("dup", "k8s-dup-1"),
		makeConfigMapWrapper("dup", "k8s-dup-2"),
		makeConfigMapWrapper("other", "k8s-other"),
	}
	got, err := FilterByName(items, "dup")
	if err != nil {
		t.Fatalf("FilterByName multiple matches: unexpected error %v", err)
	}
	if len(got) != 2 {
		t.Errorf("FilterByName multiple matches: got %d items, want 2", len(got))
	}
}

func TestFilterByName_EmptyNameEmptyList_ReturnsEmpty(t *testing.T) {
	var items []ResourceWrapper[*corev1.ConfigMap]
	got, err := FilterByName(items, "")
	if err != nil {
		t.Fatalf("FilterByName empty name+empty list: unexpected error %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FilterByName empty name+empty list: got %d items, want 0", len(got))
	}
}

// ---- GenerateResourceName ----

func TestGenerateResourceName_Deterministic(t *testing.T) {
	a := GenerateResourceName("my-org", "my-project", "my-component")
	b := GenerateResourceName("my-org", "my-project", "my-component")
	if a != b {
		t.Errorf("GenerateResourceName not deterministic: %q != %q", a, b)
	}
}

func TestGenerateResourceName_LengthWithinLimit(t *testing.T) {
	name := GenerateResourceName(
		"very-long-organization-name",
		"very-long-project-name",
		"very-long-component-name",
	)
	if len(name) > 253 {
		t.Errorf("GenerateResourceName length = %d, want <= 253", len(name))
	}
}

func TestGenerateResourceName_DifferentInputsDifferentNames(t *testing.T) {
	a := GenerateResourceName("org1", "proj1")
	b := GenerateResourceName("org2", "proj2")
	if a == b {
		t.Errorf("GenerateResourceName different inputs produced same output: %q", a)
	}
}

func TestGenerateResourceName_SinglePart(t *testing.T) {
	name := GenerateResourceName("my-resource")
	if name == "" {
		t.Error("GenerateResourceName single part: got empty string")
	}
	if len(name) > 253 {
		t.Errorf("GenerateResourceName single part length = %d, want <= 253", len(name))
	}
}

func TestGenerateResourceName_LowercaseOutput(t *testing.T) {
	name := GenerateResourceName("MyOrg", "MyProject")
	if name != strings.ToLower(name) {
		t.Errorf("GenerateResourceName not lowercase: %q", name)
	}
}

func TestGenerateResourceName_ContainsHashSuffix(t *testing.T) {
	name := GenerateResourceName("org", "project")
	// The output must contain a hash suffix separated by '-'
	// Hash is 8 hex chars at the end
	if len(name) < 9 {
		t.Errorf("GenerateResourceName output too short to contain hash: %q", name)
	}
}

// ---- DefaultIfEmpty ----

func TestDefaultIfEmpty_EmptyValue_ReturnsDefault(t *testing.T) {
	got := DefaultIfEmpty("", "fallback")
	if got != "fallback" {
		t.Errorf("DefaultIfEmpty(empty) = %q, want %q", got, "fallback")
	}
}

func TestDefaultIfEmpty_NonEmptyValue_ReturnsValue(t *testing.T) {
	got := DefaultIfEmpty("actual", "fallback")
	if got != "actual" {
		t.Errorf("DefaultIfEmpty(non-empty) = %q, want %q", got, "actual")
	}
}

func TestDefaultIfEmpty_BothEmpty_ReturnsEmpty(t *testing.T) {
	got := DefaultIfEmpty("", "")
	if got != "" {
		t.Errorf("DefaultIfEmpty(both empty) = %q, want empty string", got)
	}
}

func TestDefaultIfEmpty_EmptyDefault_ReturnsValue(t *testing.T) {
	got := DefaultIfEmpty("value", "")
	if got != "value" {
		t.Errorf("DefaultIfEmpty(empty default) = %q, want %q", got, "value")
	}
}

// ---- ResourceWrapper ----

func TestResourceWrapper_GetName(t *testing.T) {
	w := makeConfigMapWrapper("logical-name", "k8s-name")
	if w.GetName() != "logical-name" {
		t.Errorf("ResourceWrapper.GetName() = %q, want logical-name", w.GetName())
	}
}

func TestResourceWrapper_GetKubernetesName(t *testing.T) {
	w := makeConfigMapWrapper("logical-name", "k8s-name")
	if w.GetKubernetesName() != "k8s-name" {
		t.Errorf("ResourceWrapper.GetKubernetesName() = %q, want k8s-name", w.GetKubernetesName())
	}
}

func TestResourceWrapper_GetResource(t *testing.T) {
	w := makeConfigMapWrapper("logical-name", "k8s-name")
	res := w.GetResource()
	if res == nil {
		t.Fatal("ResourceWrapper.GetResource() returned nil")
	}
	if res.GetName() != "k8s-name" {
		t.Errorf("ResourceWrapper.GetResource().Name = %q, want k8s-name", res.GetName())
	}
}
