// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func TestExtractCommonMeta(t *testing.T) {
	ts := metav1.NewTime(time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-ns",
			CreationTimestamp: ts,
			Annotations: map[string]string{
				controller.AnnotationKeyDisplayName: "Test Namespace",
				controller.AnnotationKeyDescription: "A test namespace",
			},
		},
	}

	m := extractCommonMeta(ns)

	assertEqual(t, "name", m["name"], "test-ns")
	assertEqual(t, "displayName", m["displayName"], "Test Namespace")
	assertEqual(t, "description", m["description"], "A test namespace")
	assertEqual(t, "createdAt", m["createdAt"], "2025-06-15T10:30:00Z")

	if _, ok := m["namespace"]; ok {
		t.Error("namespace should not be set for cluster-scoped resources")
	}
}

func TestExtractCommonMeta_NamespacedResource(t *testing.T) {
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-project",
			Namespace: "org-ns",
		},
	}

	m := extractCommonMeta(project)

	assertEqual(t, "name", m["name"], "my-project")
	assertEqual(t, "namespace", m["namespace"], "org-ns")

	if _, ok := m["displayName"]; ok {
		t.Error("displayName should not be set when annotation is missing")
	}
	if _, ok := m["createdAt"]; ok {
		t.Error("createdAt should not be set when timestamp is zero")
	}
}

func TestReadyStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		want       string
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       "",
		},
		{
			name: "ready true",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
			want: "Ready",
		},
		{
			name: "ready false with reason",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Pending"},
			},
			want: "Pending",
		},
		{
			name: "no ready condition",
			conditions: []metav1.Condition{
				{Type: "Reconciled", Status: metav1.ConditionTrue},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readyStatus(tt.conditions)
			if got != tt.want {
				t.Errorf("readyStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConditionsSummary(t *testing.T) {
	result := conditionsSummary(nil)
	if result != nil {
		t.Error("conditionsSummary(nil) should return nil")
	}

	conditions := []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "AllGood",
			Message: "Everything is fine",
		},
		{
			Type:   "Reconciled",
			Status: metav1.ConditionTrue,
		},
	}

	result = conditionsSummary(conditions)
	if len(result) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(result))
	}

	assertEqual(t, "type", result[0]["type"], "Ready")
	assertEqual(t, "status", result[0]["status"], "True")
	assertEqual(t, "reason", result[0]["reason"], "AllGood")
	assertEqual(t, "message", result[0]["message"], "Everything is fine")

	if _, ok := result[1]["reason"]; ok {
		t.Error("empty reason should be omitted")
	}
	if _, ok := result[1]["message"]; ok {
		t.Error("empty message should be omitted")
	}
}

func TestTransformList(t *testing.T) {
	items := []openchoreov1alpha1.Project{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns"}},
	}

	result := transformList(items, projectSummary)
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	assertEqual(t, "first name", result[0]["name"], "p1")
	assertEqual(t, "second name", result[1]["name"], "p2")
}

func TestWrapTransformedList(t *testing.T) {
	items := []openchoreov1alpha1.Project{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"}},
	}

	result := wrapTransformedList("projects", items, "", projectSummary)
	projects, ok := result["projects"].([]map[string]any)
	if !ok {
		t.Fatal("expected projects key with []map[string]any")
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	if _, ok := result["next_cursor"]; ok {
		t.Error("next_cursor should not be set when empty")
	}

	result = wrapTransformedList("projects", items, "abc123", projectSummary)
	assertEqual(t, "next_cursor", result["next_cursor"], "abc123")
}

func TestMutationResult(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ns"},
	}

	m := mutationResult(ns, "created")
	assertEqual(t, "name", m["name"], "test-ns")
	assertEqual(t, "action", m["action"], "created")
	if _, ok := m["namespace"]; ok {
		t.Error("namespace should not be set for cluster-scoped resources")
	}

	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns"},
	}
	m = mutationResult(project, "patched", map[string]any{"extra": "value"})
	assertEqual(t, "namespace", m["namespace"], "ns")
	assertEqual(t, "extra", m["extra"], "value")
}

func TestProjectSummary(t *testing.T) {
	p := openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-project",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: "default-pipeline",
		},
		Status: openchoreov1alpha1.ProjectStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}

	m := projectSummary(p)
	assertEqual(t, "name", m["name"], "my-project")
	assertEqual(t, "deploymentPipelineRef", m["deploymentPipelineRef"], "default-pipeline")
	assertEqual(t, "status", m["status"], "Ready")
}

func TestComponentSummary(t *testing.T) {
	c := openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-comp",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: "my-project",
			},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Name: "deployment/my-type",
			},
			AutoDeploy: true,
		},
		Status: openchoreov1alpha1.ComponentStatus{
			LatestRelease: &openchoreov1alpha1.LatestRelease{
				Name: "v1",
			},
		},
	}

	m := componentSummary(c)
	assertEqual(t, "projectName", m["projectName"], "my-project")
	assertEqual(t, "componentType", m["componentType"], "deployment/my-type")
	if m["autoDeploy"] != true {
		t.Error("expected autoDeploy to be true")
	}
	assertEqual(t, "latestRelease", m["latestRelease"], "v1")
}

func TestEnvironmentSummary(t *testing.T) {
	e := openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			IsProduction: false,
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: "DataPlane",
				Name: "dp-1",
			},
		},
	}

	m := environmentSummary(e)
	assertEqual(t, "name", m["name"], "dev")
	if m["isProduction"] != false {
		t.Error("expected isProduction to be false")
	}
	ref, ok := m["dataPlaneRef"].(map[string]any)
	if !ok {
		t.Fatal("expected dataPlaneRef to be a map")
	}
	assertEqual(t, "dataPlaneRef.name", ref["name"], "dp-1")
}

func TestDataplaneSummary(t *testing.T) {
	dp := openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-1",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "plane-123",
		},
		Status: openchoreov1alpha1.DataPlaneStatus{
			AgentConnection: &openchoreov1alpha1.AgentConnectionStatus{
				Connected: true,
			},
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
		},
	}

	m := dataplaneSummary(dp)
	assertEqual(t, "planeID", m["planeID"], "plane-123")
	if m["agentConnected"] != true {
		t.Error("expected agentConnected to be true")
	}
	assertEqual(t, "status", m["status"], "Ready")
}

func TestRawExtensionToAny(t *testing.T) {
	result := rawExtensionToAny(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	raw := &runtime.RawExtension{Raw: []byte(`{"key":"value"}`)}
	result = rawExtensionToAny(raw)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map[string]any")
	}
	assertEqual(t, "key", m["key"], "value")
}

func TestWorkflowRunSummary(t *testing.T) {
	started := metav1.NewTime(time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC))
	completed := metav1.NewTime(time.Date(2025, 6, 15, 10, 5, 0, 0, time.UTC))

	wr := openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "build-run-1",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name: "build",
			},
		},
		Status: openchoreov1alpha1.WorkflowRunStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			},
			StartedAt:   &started,
			CompletedAt: &completed,
		},
	}

	m := workflowRunSummary(wr)
	assertEqual(t, "workflowName", m["workflowName"], "build")
	assertEqual(t, "status", m["status"], "Ready")
	assertEqual(t, "startedAt", m["startedAt"], "2025-06-15T10:00:00Z")
	assertEqual(t, "completedAt", m["completedAt"], "2025-06-15T10:05:00Z")
}

func TestReleaseBindingSummary(t *testing.T) {
	rb := openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rb-1",
			Namespace: "org-ns",
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			Environment: "dev",
			ReleaseName: "v1",
			State:       openchoreov1alpha1.ReleaseStateActive,
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Progressing"},
			},
		},
	}

	m := releaseBindingSummary(rb)
	assertEqual(t, "projectName", m["projectName"], "proj")
	assertEqual(t, "componentName", m["componentName"], "comp")
	assertEqual(t, "environment", m["environment"], "dev")
	assertEqual(t, "releaseName", m["releaseName"], "v1")
	assertEqual(t, "state", m["state"], "Active")
	assertEqual(t, "status", m["status"], "Progressing")
}

func TestResourceHealthSummary(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		{HealthStatus: openchoreov1alpha1.HealthStatusProgressing},
		{},
	}

	counts := resourceHealthSummary(resources)
	if counts["Healthy"] != 2 {
		t.Errorf("expected 2 Healthy, got %d", counts["Healthy"])
	}
	if counts["Progressing"] != 1 {
		t.Errorf("expected 1 Progressing, got %d", counts["Progressing"])
	}
	if counts["Unknown"] != 1 {
		t.Errorf("expected 1 Unknown, got %d", counts["Unknown"])
	}
}

func assertEqual(t *testing.T, field string, got, want any) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", field, got, want)
	}
}
