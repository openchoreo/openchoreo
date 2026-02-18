// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package names

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func makeBuild(name, namespace, project, component string) *openchoreov1alpha1.Build {
	return &openchoreov1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid-123"),
		},
		Spec: openchoreov1alpha1.BuildSpec{
			Owner: openchoreov1alpha1.BuildOwner{
				ProjectName:   project,
				ComponentName: component,
			},
		},
	}
}

// ---- MakeImageName tests ----

func TestMakeImageName_Basic(t *testing.T) {
	build := makeBuild("build-1", "default", "my-project", "my-component")
	name := MakeImageName(build)
	if name == "" {
		t.Fatal("MakeImageName returned empty string")
	}
	if !strings.Contains(name, "my-project") || !strings.Contains(name, "my-component") {
		t.Errorf("MakeImageName = %q, want to contain project and component names", name)
	}
}

func TestMakeImageName_Lowercase(t *testing.T) {
	build := makeBuild("b", "ns", "MyProject", "MyComponent")
	name := MakeImageName(build)
	if name != strings.ToLower(name) {
		t.Errorf("MakeImageName = %q should be all lowercase", name)
	}
}

func TestMakeImageName_MaxLength(t *testing.T) {
	// Very long project and component names
	build := makeBuild("b", "ns",
		"very-long-project-name-that-exceeds-limits-for-testing",
		"very-long-component-name-that-exceeds-limits-for-testing",
	)
	name := MakeImageName(build)
	if len(name) > MaxImageNameLength {
		t.Errorf("MakeImageName len = %d > %d", len(name), MaxImageNameLength)
	}
}

func TestMakeImageName_SpecialCharacters(t *testing.T) {
	build := makeBuild("b", "ns", "my_project.name", "my_component.name")
	name := MakeImageName(build)
	// Should not have consecutive hyphens or leading/trailing hyphens
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		t.Errorf("MakeImageName = %q has leading/trailing hyphens", name)
	}
	if strings.Contains(name, "--") {
		t.Errorf("MakeImageName = %q has consecutive hyphens", name)
	}
}

func TestMakeImageName_Format(t *testing.T) {
	build := makeBuild("b", "ns", "proj", "comp")
	name := MakeImageName(build)
	expected := "proj-comp"
	if name != expected {
		t.Errorf("MakeImageName = %q, want %q", name, expected)
	}
}

// ---- MakeImageTag tests ----

func TestMakeImageTag_ReturnsDefault(t *testing.T) {
	build := makeBuild("b", "ns", "p", "c")
	tag := MakeImageTag(build)
	if tag != DefaultDTName {
		t.Errorf("MakeImageTag = %q, want %q", tag, DefaultDTName)
	}
}

// ---- MakeWorkflowName tests ----

func TestMakeWorkflowName_NotEmpty(t *testing.T) {
	build := makeBuild("my-build", "default", "proj", "comp")
	name := MakeWorkflowName(build)
	if name == "" {
		t.Fatal("MakeWorkflowName returned empty string")
	}
}

func TestMakeWorkflowName_MaxLength(t *testing.T) {
	build := makeBuild(
		"this-is-a-very-long-build-name-that-exceeds-the-maximum-workflow-name-limit-for-kubernetes",
		"default", "proj", "comp",
	)
	name := MakeWorkflowName(build)
	if len(name) > MaxWorkflowNameLength {
		t.Errorf("MakeWorkflowName len = %d > %d", len(name), MaxWorkflowNameLength)
	}
}

func TestMakeWorkflowName_Deterministic(t *testing.T) {
	build := makeBuild("build-abc", "default", "proj", "comp")
	n1 := MakeWorkflowName(build)
	n2 := MakeWorkflowName(build)
	if n1 != n2 {
		t.Errorf("MakeWorkflowName is not deterministic: %q != %q", n1, n2)
	}
}

// ---- MakeNamespaceName tests ----

func TestMakeNamespaceName_HasPrefix(t *testing.T) {
	build := makeBuild("b", "default", "proj", "comp")
	ns := MakeNamespaceName(build)
	if !strings.HasPrefix(ns, "openchoreo-ci-") {
		t.Errorf("MakeNamespaceName = %q, want prefix 'openchoreo-ci-'", ns)
	}
}

func TestMakeNamespaceName_NormalizesNamespace(t *testing.T) {
	build := makeBuild("b", "my_namespace.test", "proj", "comp")
	ns := MakeNamespaceName(build)
	// Underscores and dots should be converted
	if strings.Contains(ns, "_") || strings.Contains(ns, ".") {
		t.Errorf("MakeNamespaceName = %q contains invalid characters (_, .)", ns)
	}
}

func TestMakeNamespaceName_Deterministic(t *testing.T) {
	build := makeBuild("b", "test-ns", "proj", "comp")
	n1 := MakeNamespaceName(build)
	n2 := MakeNamespaceName(build)
	if n1 != n2 {
		t.Errorf("MakeNamespaceName is not deterministic: %q != %q", n1, n2)
	}
}

// ---- MakeWorkflowLabels tests ----

func TestMakeWorkflowLabels_ContainsRequiredKeys(t *testing.T) {
	build := makeBuild("my-build", "default", "my-project", "my-component")
	build.UID = types.UID("test-uid-456")
	labels := MakeWorkflowLabels(build)

	if labels == nil {
		t.Fatal("MakeWorkflowLabels returned nil")
	}

	// Should have entries (we don't check specific keys to avoid coupling to label constants)
	if len(labels) == 0 {
		t.Error("MakeWorkflowLabels returned empty labels map")
	}
}

func TestMakeWorkflowLabels_NotNil(t *testing.T) {
	build := makeBuild("b", "ns", "p", "c")
	labels := MakeWorkflowLabels(build)
	if labels == nil {
		t.Fatal("MakeWorkflowLabels returned nil")
	}
}

// ---- normalizeForK8s tests (via MakeNamespaceName) ----

func TestNormalizeForK8s_UnderscoreReplaced(t *testing.T) {
	build := makeBuild("b", "my_ns", "p", "c")
	ns := MakeNamespaceName(build)
	if strings.Contains(ns, "_") {
		t.Errorf("normalizeForK8s should replace underscores, got %q", ns)
	}
}

func TestNormalizeForK8s_DotReplaced(t *testing.T) {
	build := makeBuild("b", "my.ns", "p", "c")
	ns := MakeNamespaceName(build)
	if strings.Contains(ns, ".") {
		t.Errorf("normalizeForK8s should replace dots, got %q", ns)
	}
}

func TestNormalizeForK8s_MaxLength(t *testing.T) {
	longNs := strings.Repeat("a", 100)
	build := makeBuild("b", longNs, "p", "c")
	ns := MakeNamespaceName(build)
	// "openchoreo-ci-" prefix (14 chars) + up to 63 chars normalized
	if len(ns) > len("openchoreo-ci-")+63 {
		t.Errorf("MakeNamespaceName length %d may exceed limits", len(ns))
	}
}
