// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Shared string constants used across unit tests to satisfy the goconst linter.
const (
	testProjectName   = "my-project"
	testComponentName = "my-component"
	testNamespace     = "my-ns"
	testEnvStaging    = "staging"
)

// Unit tests for pure helper functions that don't require k8s environment

func newTestReconciler() *Reconciler {
	return &Reconciler{
		Scheme: runtime.NewScheme(),
	}
}

func makeValidComponentRelease(project, component string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources: []openchoreov1alpha1.ResourceTemplate{
					{ID: "deployment"},
				},
			},
		},
	}
}

func makeValidReleaseBinding(project, component string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   project,
				ComponentName: component,
			},
		},
	}
}

// ---- validateComponentRelease tests ----

func TestValidateComponentRelease_Valid(t *testing.T) {
	r := newTestReconciler()
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	rb := makeValidReleaseBinding(testProjectName, testComponentName)

	if err := r.validateComponentRelease(cr, rb); err != nil {
		t.Errorf("validateComponentRelease returned unexpected error: %v", err)
	}
}

func TestValidateComponentRelease_NilResources(t *testing.T) {
	r := newTestReconciler()

	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "comp",
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources:    nil, // nil resources
			},
		},
	}
	rb := makeValidReleaseBinding("proj", "comp")

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when resources is nil")
	}
}

func TestValidateComponentRelease_MissingProjectName(t *testing.T) {
	r := newTestReconciler()

	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "", // missing
				ComponentName: "comp",
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
			},
		},
	}
	rb := makeValidReleaseBinding("", "comp")

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when projectName is empty")
	}
}

func TestValidateComponentRelease_MissingComponentName(t *testing.T) {
	r := newTestReconciler()

	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   "proj",
				ComponentName: "", // missing
			},
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
				Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "deployment"}},
			},
		},
	}
	rb := makeValidReleaseBinding("proj", "")

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when componentName is empty")
	}
}

func TestValidateComponentRelease_OwnerMismatch_Project(t *testing.T) {
	r := newTestReconciler()
	cr := makeValidComponentRelease("project-A", "comp")
	rb := makeValidReleaseBinding("project-B", "comp") // different project

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when project names don't match")
	}
}

func TestValidateComponentRelease_OwnerMismatch_Component(t *testing.T) {
	r := newTestReconciler()
	cr := makeValidComponentRelease("proj", "comp-A")
	rb := makeValidReleaseBinding("proj", "comp-B") // different component

	err := r.validateComponentRelease(cr, rb)
	if err == nil {
		t.Error("validateComponentRelease should return error when component names don't match")
	}
}

// ---- NewReleaseBindingFinalizingCondition ----

func TestNewReleaseBindingFinalizingCondition(t *testing.T) {
	cond := NewReleaseBindingFinalizingCondition(5)
	if cond.Type != string(ConditionFinalizing) {
		t.Errorf("Type = %q, want %q", cond.Type, ConditionFinalizing)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.ObservedGeneration != 5 {
		t.Errorf("ObservedGeneration = %d, want 5", cond.ObservedGeneration)
	}
}

// ---- release name helpers ----

func TestMakeDataPlaneReleaseName_Format(t *testing.T) {
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	rb := makeValidReleaseBinding(testProjectName, testComponentName)
	rb.Spec.Environment = testEnvStaging

	got := makeDataPlaneReleaseName(cr, rb)
	want := testComponentName + "-" + testEnvStaging
	if got != want {
		t.Errorf("makeDataPlaneReleaseName = %q, want %q", got, want)
	}
}

func TestMakeObservabilityReleaseName_Format(t *testing.T) {
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	rb := makeValidReleaseBinding(testProjectName, testComponentName)
	rb.Spec.Environment = testEnvStaging

	got := makeObservabilityReleaseName(cr, rb)
	want := testComponentName + "-" + testEnvStaging + "-observability"
	if got != want {
		t.Errorf("makeObservabilityReleaseName = %q, want %q", got, want)
	}
}

// ---- generateResourceID tests ----

func TestGenerateResourceID_KindAndName(t *testing.T) {
	r := newTestReconciler()
	resource := map[string]any{
		"kind": "Deployment",
		"metadata": map[string]any{
			"name": "my-app",
		},
	}
	got := r.generateResourceID(resource, 0)
	want := "deployment-my-app"
	if got != want {
		t.Errorf("generateResourceID = %q, want %q", got, want)
	}
}

func TestGenerateResourceID_MissingKind_FallsBackToIndex(t *testing.T) {
	r := newTestReconciler()
	resource := map[string]any{
		"metadata": map[string]any{"name": "my-app"},
	}
	got := r.generateResourceID(resource, 5)
	want := "resource-5"
	if got != want {
		t.Errorf("generateResourceID with missing kind = %q, want %q", got, want)
	}
}

func TestGenerateResourceID_MissingName_FallsBackToIndex(t *testing.T) {
	r := newTestReconciler()
	resource := map[string]any{
		"kind": "Service",
	}
	got := r.generateResourceID(resource, 3)
	want := "resource-3"
	if got != want {
		t.Errorf("generateResourceID with missing name = %q, want %q", got, want)
	}
}

func TestGenerateResourceID_EmptyResource_FallsBackToIndex(t *testing.T) {
	r := newTestReconciler()
	got := r.generateResourceID(map[string]any{}, 7)
	want := "resource-7"
	if got != want {
		t.Errorf("generateResourceID for empty resource = %q, want %q", got, want)
	}
}

// ---- buildComponentFromRelease tests ----

func TestBuildComponentFromRelease_WithoutProfile(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   testProjectName,
				ComponentName: testComponentName,
			},
		},
	}

	comp := buildComponentFromRelease(cr)

	if comp.Name != testComponentName {
		t.Errorf("Name = %q, want %q", comp.Name, testComponentName)
	}
	if comp.Namespace != testNamespace {
		t.Errorf("Namespace = %q, want %q", comp.Namespace, testNamespace)
	}
	if comp.Spec.Owner.ProjectName != testProjectName {
		t.Errorf("Owner.ProjectName = %q, want %q", comp.Spec.Owner.ProjectName, testProjectName)
	}
	if comp.Spec.Parameters != nil {
		t.Error("Parameters should be nil when ComponentProfile is nil")
	}
	if len(comp.Spec.Traits) != 0 {
		t.Error("Traits should be empty when ComponentProfile is nil")
	}
}

func TestBuildComponentFromRelease_WithProfile(t *testing.T) {
	raw := []byte(`{"replicas":3}`)
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   testProjectName,
				ComponentName: testComponentName,
			},
			ComponentProfile: &openchoreov1alpha1.ComponentProfile{
				Parameters: &runtime.RawExtension{Raw: raw},
				Traits: []openchoreov1alpha1.ComponentTrait{
					{Name: "autoscaler"},
				},
			},
		},
	}

	comp := buildComponentFromRelease(cr)

	if comp.Spec.Parameters == nil {
		t.Error("Parameters should be propagated from ComponentProfile")
	}
	if len(comp.Spec.Traits) != 1 {
		t.Errorf("Traits length = %d, want 1", len(comp.Spec.Traits))
	}
}

// ---- buildWorkloadFromRelease tests ----

func TestBuildWorkloadFromRelease_OwnerAndNamespace(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   testProjectName,
				ComponentName: testComponentName,
			},
		},
	}

	wl := buildWorkloadFromRelease(cr)

	if wl.Namespace != testNamespace {
		t.Errorf("Namespace = %q, want %q", wl.Namespace, testNamespace)
	}
	if wl.Spec.Owner.ProjectName != testProjectName {
		t.Errorf("Owner.ProjectName = %q, want %q", wl.Spec.Owner.ProjectName, testProjectName)
	}
	if wl.Spec.Owner.ComponentName != testComponentName {
		t.Errorf("Owner.ComponentName = %q, want %q", wl.Spec.Owner.ComponentName, testComponentName)
	}
}

// ---- buildComponentTypeFromRelease tests ----

func TestBuildComponentTypeFromRelease_SpecIsCopied(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeSpec{
				WorkloadType: "statefulset",
				Resources:    []openchoreov1alpha1.ResourceTemplate{{ID: "sts"}},
			},
		},
	}

	ct := buildComponentTypeFromRelease(cr)

	if ct.Namespace != testNamespace {
		t.Errorf("Namespace = %q, want %q", ct.Namespace, testNamespace)
	}
	if ct.Spec.WorkloadType != "statefulset" {
		t.Errorf("WorkloadType = %q, want %q", ct.Spec.WorkloadType, "statefulset")
	}
	if len(ct.Spec.Resources) != 1 || ct.Spec.Resources[0].ID != "sts" {
		t.Errorf("Resources mismatch: %+v", ct.Spec.Resources)
	}
}

// ---- buildTraitsFromRelease tests ----

func TestBuildTraitsFromRelease_NilTraits_ReturnsNil(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{}
	traits := buildTraitsFromRelease(cr)
	if traits != nil {
		t.Errorf("expected nil, got %v", traits)
	}
}

func TestBuildTraitsFromRelease_EmptyMap_ReturnsNil(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Traits: map[string]openchoreov1alpha1.TraitSpec{},
		},
	}
	traits := buildTraitsFromRelease(cr)
	if traits != nil {
		t.Errorf("expected nil for empty map, got %v", traits)
	}
}

func TestBuildTraitsFromRelease_WithTraits_NamespaceAndCountCorrect(t *testing.T) {
	cr := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Traits: map[string]openchoreov1alpha1.TraitSpec{
				"autoscaler": {},
				"ingress":    {},
			},
		},
	}

	traits := buildTraitsFromRelease(cr)

	if len(traits) != 2 {
		t.Errorf("expected 2 traits, got %d", len(traits))
	}
	for _, trait := range traits {
		if trait.Namespace != testNamespace {
			t.Errorf("trait %q namespace = %q, want %q", trait.Name, trait.Namespace, testNamespace)
		}
		if trait.Name != "autoscaler" && trait.Name != "ingress" {
			t.Errorf("unexpected trait name %q", trait.Name)
		}
	}
}

// ---- buildMetadataContext tests ----

func TestBuildMetadataContext_FieldsPopulated(t *testing.T) {
	r := newTestReconciler()

	componentRelease := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   testProjectName,
				ComponentName: testComponentName,
			},
		},
	}
	component := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			UID: "comp-uid-123",
		},
	}
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			UID: "proj-uid-456",
		},
	}
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-dataplane",
			UID:  "dp-uid-789",
		},
	}
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			UID: "env-uid-101",
		},
	}
	environmentName := testEnvStaging

	ctx := r.buildMetadataContext(componentRelease, component, project, dataPlane, environment, environmentName)

	if ctx.ComponentName != testComponentName {
		t.Errorf("ComponentName = %q, want %q", ctx.ComponentName, testComponentName)
	}
	if ctx.ProjectName != testProjectName {
		t.Errorf("ProjectName = %q, want %q", ctx.ProjectName, testProjectName)
	}
	if ctx.DataPlaneName != "my-dataplane" {
		t.Errorf("DataPlaneName = %q, want %q", ctx.DataPlaneName, "my-dataplane")
	}
	if ctx.EnvironmentName != testEnvStaging {
		t.Errorf("EnvironmentName = %q, want %q", ctx.EnvironmentName, testEnvStaging)
	}
	if ctx.ComponentUID != "comp-uid-123" {
		t.Errorf("ComponentUID = %q, want %q", ctx.ComponentUID, "comp-uid-123")
	}
	if ctx.ProjectUID != "proj-uid-456" {
		t.Errorf("ProjectUID = %q, want %q", ctx.ProjectUID, "proj-uid-456")
	}
	if ctx.DataPlaneUID != "dp-uid-789" {
		t.Errorf("DataPlaneUID = %q, want %q", ctx.DataPlaneUID, "dp-uid-789")
	}
	if ctx.EnvironmentUID != "env-uid-101" {
		t.Errorf("EnvironmentUID = %q, want %q", ctx.EnvironmentUID, "env-uid-101")
	}
	if ctx.Name == "" {
		t.Error("Name should not be empty")
	}
	if ctx.Namespace == "" {
		t.Error("Namespace should not be empty")
	}
	if len(ctx.Labels) == 0 {
		t.Error("Labels should not be empty")
	}
	if ctx.Annotations == nil {
		t.Error("Annotations should not be nil")
	}
	if len(ctx.PodSelectors) == 0 {
		t.Error("PodSelectors should not be empty")
	}
}

// ─── extractWorkloadType ───────────────────────────────────────────────────────

func TestExtractWorkloadType_KnownTypes(t *testing.T) {
	tests := []struct {
		input string
		want  WorkloadType
	}{
		{"deployment/http-service", WorkloadTypeDeployment},
		{"statefulset/db", WorkloadTypeStatefulSet},
		{"cronjob/nightly-task", WorkloadTypeCronJob},
		{"job/migration", WorkloadTypeJob},
		{"proxy/my-proxy", WorkloadTypeProxy},
		{"", WorkloadTypeUnknown},
		{"unknown/something", WorkloadTypeUnknown},
		{"deployment", WorkloadTypeDeployment}, // no slash — first part only
	}
	for _, tc := range tests {
		got := extractWorkloadType(tc.input)
		if got != tc.want {
			t.Errorf("extractWorkloadType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── isPrimaryWorkload ────────────────────────────────────────────────────────

func TestIsPrimaryWorkload(t *testing.T) {
	tests := []struct {
		gvk          schema.GroupVersionKind
		workloadType WorkloadType
		want         bool
	}{
		{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, WorkloadTypeDeployment, true},
		{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, WorkloadTypeStatefulSet, true},
		{schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, WorkloadTypeCronJob, true},
		{schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, WorkloadTypeJob, true},
		// wrong kind for workload type
		{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, WorkloadTypeStatefulSet, false},
		// proxy has no primary workload
		{schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, WorkloadTypeProxy, false},
	}
	for _, tc := range tests {
		got := isPrimaryWorkload(tc.gvk, tc.workloadType)
		if got != tc.want {
			t.Errorf("isPrimaryWorkload(%v, %q) = %v, want %v", tc.gvk, tc.workloadType, got, tc.want)
		}
	}
}

// ─── categorizeResource ───────────────────────────────────────────────────────

func TestCategorizeResource(t *testing.T) {
	tests := []struct {
		gvk          schema.GroupVersionKind
		workloadType WorkloadType
		want         ResourceCategory
	}{
		// Primary workload matches workloadType
		{schema.GroupVersionKind{Group: "apps", Kind: "Deployment"}, WorkloadTypeDeployment, CategoryPrimaryWorkload},
		{schema.GroupVersionKind{Group: "apps", Kind: "StatefulSet"}, WorkloadTypeStatefulSet, CategoryPrimaryWorkload},
		{schema.GroupVersionKind{Group: "batch", Kind: "Job"}, WorkloadTypeJob, CategoryPrimaryWorkload},
		{schema.GroupVersionKind{Group: "batch", Kind: "CronJob"}, WorkloadTypeCronJob, CategoryPrimaryWorkload},
		// Supporting resources
		{schema.GroupVersionKind{Group: "", Kind: "Service"}, WorkloadTypeDeployment, CategorySupporting},
		{schema.GroupVersionKind{Group: "", Kind: "PersistentVolumeClaim"}, WorkloadTypeDeployment, CategorySupporting},
		// No-status resources
		{schema.GroupVersionKind{Group: "", Kind: "ConfigMap"}, WorkloadTypeDeployment, CategoryNoStatus},
		{schema.GroupVersionKind{Group: "", Kind: "Secret"}, WorkloadTypeDeployment, CategoryNoStatus},
		{schema.GroupVersionKind{Group: "", Kind: "ServiceAccount"}, WorkloadTypeDeployment, CategoryNoStatus},
		// Operational resources
		{schema.GroupVersionKind{Group: "autoscaling", Kind: "HorizontalPodAutoscaler"}, WorkloadTypeDeployment, CategoryOperational},
		{schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"}, WorkloadTypeDeployment, CategoryOperational},
		// Unknown → Operational
		{schema.GroupVersionKind{Group: "custom.io", Kind: "FooBar"}, WorkloadTypeDeployment, CategoryOperational},
	}
	for _, tc := range tests {
		got := categorizeResource(tc.gvk, tc.workloadType)
		if got != tc.want {
			t.Errorf("categorizeResource(%v, %q) = %q, want %q", tc.gvk, tc.workloadType, got, tc.want)
		}
	}
}

// ─── aggregateResourceStatus ─────────────────────────────────────────────────

func TestAggregateResourceStatus(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		{HealthStatus: openchoreov1alpha1.HealthStatusProgressing},
		{HealthStatus: openchoreov1alpha1.HealthStatusDegraded},
		{HealthStatus: openchoreov1alpha1.HealthStatusSuspended},
		{HealthStatus: openchoreov1alpha1.HealthStatusUnknown},
	}
	got := aggregateResourceStatus(resources)
	if got.Total != 6 {
		t.Errorf("Total = %d, want 6", got.Total)
	}
	if got.Healthy != 2 {
		t.Errorf("Healthy = %d, want 2", got.Healthy)
	}
	if got.Progressing != 1 {
		t.Errorf("Progressing = %d, want 1", got.Progressing)
	}
	if got.Degraded != 1 {
		t.Errorf("Degraded = %d, want 1", got.Degraded)
	}
	if got.Suspended != 1 {
		t.Errorf("Suspended = %d, want 1", got.Suspended)
	}
	if got.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", got.Unknown)
	}

	// Empty slice
	empty := aggregateResourceStatus(nil)
	if empty.Total != 0 {
		t.Errorf("empty Total = %d, want 0", empty.Total)
	}
}

// ─── evaluateDeploymentStatus ────────────────────────────────────────────────

func deploymentResource(status openchoreov1alpha1.HealthStatus) openchoreov1alpha1.ResourceStatus {
	return openchoreov1alpha1.ResourceStatus{
		Group:        "apps",
		Version:      "v1",
		Kind:         "Deployment",
		Name:         "test-deploy",
		HealthStatus: status,
	}
}

func serviceResource(status openchoreov1alpha1.HealthStatus) openchoreov1alpha1.ResourceStatus {
	return openchoreov1alpha1.ResourceStatus{
		Group:        "",
		Version:      "v1",
		Kind:         "Service",
		Name:         "test-svc",
		HealthStatus: status,
	}
}

func hpaResource(status openchoreov1alpha1.HealthStatus) openchoreov1alpha1.ResourceStatus {
	return openchoreov1alpha1.ResourceStatus{
		Group:        "autoscaling",
		Version:      "v2",
		Kind:         "HorizontalPodAutoscaler",
		Name:         "test-hpa",
		HealthStatus: status,
	}
}

func TestEvaluateDeploymentStatus_NoPrimaryWorkload(t *testing.T) {
	ready, reason, _ := evaluateDeploymentStatus([]openchoreov1alpha1.ResourceStatus{}, WorkloadTypeDeployment)
	if ready {
		t.Error("should not be ready when no primary workload")
	}
	if reason != string(ReasonResourcesProgressing) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesProgressing)
	}
}

func TestEvaluateDeploymentStatus_PrimaryDegraded(t *testing.T) {
	ready, reason, _ := evaluateDeploymentStatus(
		[]openchoreov1alpha1.ResourceStatus{deploymentResource(openchoreov1alpha1.HealthStatusDegraded)},
		WorkloadTypeDeployment,
	)
	if ready {
		t.Error("should not be ready when primary workload is degraded")
	}
	if reason != string(ReasonResourcesDegraded) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesDegraded)
	}
}

func TestEvaluateDeploymentStatus_PrimaryProgressing(t *testing.T) {
	ready, reason, _ := evaluateDeploymentStatus(
		[]openchoreov1alpha1.ResourceStatus{deploymentResource(openchoreov1alpha1.HealthStatusProgressing)},
		WorkloadTypeDeployment,
	)
	if ready {
		t.Error("should not be ready when primary workload is progressing")
	}
	if reason != string(ReasonResourcesProgressing) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesProgressing)
	}
}

func TestEvaluateDeploymentStatus_PrimaryUnknown(t *testing.T) {
	ready, reason, _ := evaluateDeploymentStatus(
		[]openchoreov1alpha1.ResourceStatus{deploymentResource(openchoreov1alpha1.HealthStatusUnknown)},
		WorkloadTypeDeployment,
	)
	if ready {
		t.Error("should not be ready when primary workload is unknown")
	}
	if reason != string(ReasonResourcesUnknown) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesUnknown)
	}
}

func TestEvaluateDeploymentStatus_PrimarySuspended(t *testing.T) {
	ready, reason, _ := evaluateDeploymentStatus(
		[]openchoreov1alpha1.ResourceStatus{deploymentResource(openchoreov1alpha1.HealthStatusSuspended)},
		WorkloadTypeDeployment,
	)
	if !ready {
		t.Error("suspended deployment should be considered ready (scaled to 0)")
	}
	if reason != string(ReasonReadyWithSuspendedResources) {
		t.Errorf("reason = %q, want %q", reason, ReasonReadyWithSuspendedResources)
	}
}

func TestEvaluateDeploymentStatus_PrimaryHealthy_AllHealthy(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		deploymentResource(openchoreov1alpha1.HealthStatusHealthy),
		serviceResource(openchoreov1alpha1.HealthStatusHealthy),
	}
	ready, reason, _ := evaluateDeploymentStatus(resources, WorkloadTypeDeployment)
	if !ready {
		t.Error("should be ready when primary and all supporting resources are healthy")
	}
	if reason != string(ReasonReady) {
		t.Errorf("reason = %q, want %q", reason, ReasonReady)
	}
}

func TestEvaluateDeploymentStatus_PrimaryHealthy_SupportingDegraded(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		deploymentResource(openchoreov1alpha1.HealthStatusHealthy),
		serviceResource(openchoreov1alpha1.HealthStatusDegraded),
	}
	ready, reason, _ := evaluateDeploymentStatus(resources, WorkloadTypeDeployment)
	if ready {
		t.Error("should not be ready when supporting resource is degraded")
	}
	if reason != string(ReasonResourcesDegraded) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesDegraded)
	}
}

func TestEvaluateDeploymentStatus_PrimaryHealthy_OperationalDegraded(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		deploymentResource(openchoreov1alpha1.HealthStatusHealthy),
		hpaResource(openchoreov1alpha1.HealthStatusDegraded),
	}
	ready, reason, _ := evaluateDeploymentStatus(resources, WorkloadTypeDeployment)
	if ready {
		t.Error("should not be ready when operational resource is degraded")
	}
	if reason != string(ReasonResourcesDegraded) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesDegraded)
	}
}

// ─── evaluateCronJobStatus ───────────────────────────────────────────────────

func cronJobResource(status openchoreov1alpha1.HealthStatus) openchoreov1alpha1.ResourceStatus {
	return openchoreov1alpha1.ResourceStatus{
		Group:        "batch",
		Version:      "v1",
		Kind:         "CronJob",
		Name:         "test-cj",
		HealthStatus: status,
	}
}

func TestEvaluateCronJobStatus_AllPathsAndNoPrimary(t *testing.T) {
	tests := []struct {
		status     openchoreov1alpha1.HealthStatus
		wantReady  bool
		wantReason string
	}{
		{openchoreov1alpha1.HealthStatusDegraded, false, string(ReasonResourcesDegraded)},
		{openchoreov1alpha1.HealthStatusUnknown, false, string(ReasonResourcesUnknown)},
		{openchoreov1alpha1.HealthStatusSuspended, true, string(ReasonCronJobSuspended)},
		{openchoreov1alpha1.HealthStatusProgressing, true, string(ReasonCronJobScheduled)},
		{openchoreov1alpha1.HealthStatusHealthy, true, string(ReasonCronJobScheduled)},
	}
	for _, tc := range tests {
		ready, reason, _ := evaluateCronJobStatus(
			[]openchoreov1alpha1.ResourceStatus{cronJobResource(tc.status)},
			WorkloadTypeCronJob,
		)
		if ready != tc.wantReady {
			t.Errorf("status=%q: ready=%v, want %v", tc.status, ready, tc.wantReady)
		}
		if reason != tc.wantReason {
			t.Errorf("status=%q: reason=%q, want %q", tc.status, reason, tc.wantReason)
		}
	}

	// No primary CronJob
	ready, reason, _ := evaluateCronJobStatus([]openchoreov1alpha1.ResourceStatus{}, WorkloadTypeCronJob)
	if ready || reason != string(ReasonResourcesProgressing) {
		t.Errorf("no primary: ready=%v reason=%q, want false/%q", ready, reason, ReasonResourcesProgressing)
	}
}

// ─── evaluateJobStatus ────────────────────────────────────────────────────────

func jobResource(status openchoreov1alpha1.HealthStatus) openchoreov1alpha1.ResourceStatus {
	return openchoreov1alpha1.ResourceStatus{
		Group:        "batch",
		Version:      "v1",
		Kind:         "Job",
		Name:         "test-job",
		HealthStatus: status,
	}
}

func TestEvaluateJobStatus_AllPathsAndNoPrimary(t *testing.T) {
	tests := []struct {
		status     openchoreov1alpha1.HealthStatus
		wantReady  bool
		wantReason string
	}{
		{openchoreov1alpha1.HealthStatusDegraded, false, string(ReasonJobFailed)},
		{openchoreov1alpha1.HealthStatusUnknown, false, string(ReasonResourcesUnknown)},
		{openchoreov1alpha1.HealthStatusProgressing, false, string(ReasonJobRunning)},
		{openchoreov1alpha1.HealthStatusSuspended, false, string(ReasonJobRunning)},
		{openchoreov1alpha1.HealthStatusHealthy, true, string(ReasonJobCompleted)},
	}
	for _, tc := range tests {
		ready, reason, _ := evaluateJobStatus(
			[]openchoreov1alpha1.ResourceStatus{jobResource(tc.status)},
			WorkloadTypeJob,
		)
		if ready != tc.wantReady {
			t.Errorf("status=%q: ready=%v, want %v", tc.status, ready, tc.wantReady)
		}
		if reason != tc.wantReason {
			t.Errorf("status=%q: reason=%q, want %q", tc.status, reason, tc.wantReason)
		}
	}

	// No primary Job
	ready, reason, _ := evaluateJobStatus([]openchoreov1alpha1.ResourceStatus{}, WorkloadTypeJob)
	if ready || reason != string(ReasonResourcesProgressing) {
		t.Errorf("no primary: ready=%v reason=%q, want false/%q", ready, reason, ReasonResourcesProgressing)
	}
}

// ─── evaluateGenericStatus ────────────────────────────────────────────────────

func TestEvaluateGenericStatus(t *testing.T) {
	// Empty → ready
	ready, reason, _ := evaluateGenericStatus(nil)
	if !ready || reason != string(ReasonReady) {
		t.Errorf("empty: ready=%v reason=%q, want true/Ready", ready, reason)
	}

	// All healthy → ready
	ready, reason, _ = evaluateGenericStatus([]openchoreov1alpha1.ResourceStatus{
		{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		{HealthStatus: openchoreov1alpha1.HealthStatusSuspended},
	})
	if !ready || reason != string(ReasonReady) {
		t.Errorf("all healthy: ready=%v reason=%q, want true/Ready", ready, reason)
	}

	// Any degraded → not ready
	ready, reason, _ = evaluateGenericStatus([]openchoreov1alpha1.ResourceStatus{
		{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
		{HealthStatus: openchoreov1alpha1.HealthStatusDegraded},
	})
	if ready || reason != string(ReasonResourcesDegraded) {
		t.Errorf("degraded: ready=%v reason=%q, want false/Degraded", ready, reason)
	}

	// Any progressing → not ready
	ready, reason, _ = evaluateGenericStatus([]openchoreov1alpha1.ResourceStatus{
		{HealthStatus: openchoreov1alpha1.HealthStatusProgressing},
	})
	if ready || reason != string(ReasonResourcesProgressing) {
		t.Errorf("progressing: ready=%v reason=%q, want false/Progressing", ready, reason)
	}

	// Any unknown → not ready
	ready, reason, _ = evaluateGenericStatus([]openchoreov1alpha1.ResourceStatus{
		{HealthStatus: openchoreov1alpha1.HealthStatusUnknown},
	})
	if ready || reason != string(ReasonResourcesUnknown) {
		t.Errorf("unknown: ready=%v reason=%q, want false/Unknown", ready, reason)
	}
}

// ─── convertToReleaseResources ───────────────────────────────────────────────

func TestConvertToReleaseResources_EmptyInput(t *testing.T) {
	r := newTestReconciler()
	result, err := r.convertToReleaseResources(nil)
	if err != nil {
		t.Errorf("unexpected error for nil input: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestConvertToReleaseResources_SingleResource(t *testing.T) {
	r := newTestReconciler()
	resources := []map[string]any{
		{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name": "my-app",
			},
			"spec": map[string]any{
				"replicas": 3,
			},
		},
	}

	result, err := r.convertToReleaseResources(resources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(result))
	}

	// Verify ID is generated from kind and name
	if result[0].ID != "deployment-my-app" {
		t.Errorf("ID = %q, want %q", result[0].ID, "deployment-my-app")
	}

	// Verify Object contains valid JSON
	if result[0].Object == nil || len(result[0].Object.Raw) == 0 {
		t.Fatal("Object.Raw should not be nil or empty")
	}

	// Verify the JSON can be unmarshalled back
	var decoded map[string]any
	if err := json.Unmarshal(result[0].Object.Raw, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Object.Raw: %v", err)
	}
	if decoded["kind"] != "Deployment" {
		t.Errorf("decoded kind = %q, want %q", decoded["kind"], "Deployment")
	}
}

func TestConvertToReleaseResources_MultipleResources(t *testing.T) {
	r := newTestReconciler()
	resources := []map[string]any{
		{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": "deploy-1"},
		},
		{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]any{"name": "svc-1"},
		},
		{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "cm-1"},
		},
	}

	result, err := r.convertToReleaseResources(resources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(result))
	}

	expectedIDs := []string{"deployment-deploy-1", "service-svc-1", "configmap-cm-1"}
	for i, want := range expectedIDs {
		if result[i].ID != want {
			t.Errorf("result[%d].ID = %q, want %q", i, result[i].ID, want)
		}
	}
}

func TestConvertToReleaseResources_FallbackID(t *testing.T) {
	r := newTestReconciler()
	// Resource without kind or name → ID falls back to "resource-{index}"
	resources := []map[string]any{
		{"data": "something"},
	}

	result, err := r.convertToReleaseResources(resources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].ID != "resource-0" {
		t.Errorf("ID = %q, want %q", result[0].ID, "resource-0")
	}
}

// ─── setReleaseSyncedCondition ───────────────────────────────────────────────

func makeReleaseBindingForConditions() *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-rb",
			Namespace:  "default",
			Generation: 1,
		},
	}
}

func TestSetReleaseSyncedCondition_Created(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	r.setReleaseSyncedCondition(rb, "my-release", controllerutil.OperationResultCreated, 3,
		observabilityReleaseResult{managed: false})

	cond := findCondition(rb.Status.Conditions, string(ConditionReleaseSynced))
	if cond == nil {
		t.Fatal("ReleaseSynced condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonReleaseCreated) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReleaseCreated)
	}
}

func TestSetReleaseSyncedCondition_Updated(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	r.setReleaseSyncedCondition(rb, "my-release", controllerutil.OperationResultUpdated, 5,
		observabilityReleaseResult{managed: false})

	cond := findCondition(rb.Status.Conditions, string(ConditionReleaseSynced))
	if cond == nil {
		t.Fatal("ReleaseSynced condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonReleaseCreated) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReleaseCreated)
	}
}

func TestSetReleaseSyncedCondition_None(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	r.setReleaseSyncedCondition(rb, "my-release", controllerutil.OperationResultNone, 3,
		observabilityReleaseResult{managed: false})

	cond := findCondition(rb.Status.Conditions, string(ConditionReleaseSynced))
	if cond == nil {
		t.Fatal("ReleaseSynced condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonReleaseSynced) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReleaseSynced)
	}
}

func TestSetReleaseSyncedCondition_WithObservability(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	r.setReleaseSyncedCondition(rb, "my-release", controllerutil.OperationResultCreated, 3,
		observabilityReleaseResult{
			managed:       true,
			releaseName:   "obs-release",
			operation:     controllerutil.OperationResultCreated,
			resourceCount: 2,
		})

	cond := findCondition(rb.Status.Conditions, string(ConditionReleaseSynced))
	if cond == nil {
		t.Fatal("ReleaseSynced condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonReleaseCreated) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReleaseCreated)
	}
	// Message should mention observability release
	if cond.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestSetReleaseSyncedCondition_NoneWithObservability(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	r.setReleaseSyncedCondition(rb, "my-release", controllerutil.OperationResultNone, 3,
		observabilityReleaseResult{
			managed:     true,
			releaseName: "obs-release",
			operation:   controllerutil.OperationResultNone,
		})

	cond := findCondition(rb.Status.Conditions, string(ConditionReleaseSynced))
	if cond == nil {
		t.Fatal("ReleaseSynced condition should be set")
	}
	if cond.Reason != string(ReasonReleaseSynced) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReleaseSynced)
	}
}

// ─── setReadyCondition ───────────────────────────────────────────────────────

func setConditionOnRB(rb *openchoreov1alpha1.ReleaseBinding, condType string, status metav1.ConditionStatus, reason, message string) {
	for i := range rb.Status.Conditions {
		if rb.Status.Conditions[i].Type == condType {
			rb.Status.Conditions[i].Status = status
			rb.Status.Conditions[i].Reason = reason
			rb.Status.Conditions[i].Message = message
			return
		}
	}
	rb.Status.Conditions = append(rb.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

func TestSetReadyCondition_BothTrue_Ready(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
	setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionTrue, string(ReasonReady), "ready")

	r.setReadyCondition(rb)

	cond := findCondition(rb.Status.Conditions, string(ConditionReady))
	if cond == nil {
		t.Fatal("Ready condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonReady) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReady)
	}
}

func TestSetReadyCondition_ReleaseSyncedFalse_NotReady(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionFalse, string(ReasonComponentReleaseNotFound), "CR not found")
	setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionTrue, string(ReasonReady), "ready")

	r.setReadyCondition(rb)

	cond := findCondition(rb.Status.Conditions, string(ConditionReady))
	if cond == nil {
		t.Fatal("Ready condition should be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonComponentReleaseNotFound) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonComponentReleaseNotFound)
	}
}

func TestSetReadyCondition_ResourcesReadyFalse_NotReady(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
	setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionFalse, string(ReasonResourcesDegraded), "degraded")

	r.setReadyCondition(rb)

	cond := findCondition(rb.Status.Conditions, string(ConditionReady))
	if cond == nil {
		t.Fatal("Ready condition should be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonResourcesDegraded) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonResourcesDegraded)
	}
}

func TestSetReadyCondition_NoConditionsSet_NotReady(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	r.setReadyCondition(rb)

	cond := findCondition(rb.Status.Conditions, string(ConditionReady))
	if cond == nil {
		t.Fatal("Ready condition should be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonReleaseSynced) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReleaseSynced)
	}
}

func TestSetReadyCondition_ReleaseSyncedTrue_NoResourcesReady_NotReady(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()

	setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
	// No ResourcesReady condition set

	r.setReadyCondition(rb)

	cond := findCondition(rb.Status.Conditions, string(ConditionReady))
	if cond == nil {
		t.Fatal("Ready condition should be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonResourcesProgressing) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonResourcesProgressing)
	}
}

// ─── setResourcesReadyStatus ─────────────────────────────────────────────────

// testContext returns a context with a logger suitable for unit tests.
func testContext() context.Context {
	return log.IntoContext(context.Background(), zap.New(zap.WriteTo(nil)))
}

func TestSetResourcesReadyStatus_NoResources_Progressing(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		// Status.Resources is empty
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/my-svc"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonResourcesProgressing) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonResourcesProgressing)
	}
}

func TestSetResourcesReadyStatus_DeploymentHealthy(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		Status: openchoreov1alpha1.ReleaseStatus{
			Resources: []openchoreov1alpha1.ResourceStatus{
				{Group: "apps", Version: "v1", Kind: "Deployment", Name: "app", HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/my-svc"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonReady) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonReady)
	}
}

func TestSetResourcesReadyStatus_StatefulSetDegraded(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		Status: openchoreov1alpha1.ReleaseStatus{
			Resources: []openchoreov1alpha1.ResourceStatus{
				{Group: "apps", Version: "v1", Kind: "StatefulSet", Name: "db", HealthStatus: openchoreov1alpha1.HealthStatusDegraded},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "statefulset/my-db"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("Status = %q, want False", cond.Status)
	}
	if cond.Reason != string(ReasonResourcesDegraded) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonResourcesDegraded)
	}
}

func TestSetResourcesReadyStatus_CronJobScheduled(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		Status: openchoreov1alpha1.ReleaseStatus{
			Resources: []openchoreov1alpha1.ResourceStatus{
				{Group: "batch", Version: "v1", Kind: "CronJob", Name: "nightly", HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "cronjob/nightly"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonCronJobScheduled) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonCronJobScheduled)
	}
}

func TestSetResourcesReadyStatus_JobCompleted(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		Status: openchoreov1alpha1.ReleaseStatus{
			Resources: []openchoreov1alpha1.ResourceStatus{
				{Group: "batch", Version: "v1", Kind: "Job", Name: "migration", HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "job/migration"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
	if cond.Reason != string(ReasonJobCompleted) {
		t.Errorf("Reason = %q, want %q", cond.Reason, ReasonJobCompleted)
	}
}

func TestSetResourcesReadyStatus_ProxyGenericEvaluation(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		Status: openchoreov1alpha1.ReleaseStatus{
			Resources: []openchoreov1alpha1.ResourceStatus{
				{Group: "", Version: "v1", Kind: "Service", Name: "proxy-svc", HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "proxy/my-proxy"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
}

func TestSetResourcesReadyStatus_UnknownWorkloadType(t *testing.T) {
	r := newTestReconciler()
	rb := makeReleaseBindingForConditions()
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{Name: "test-release"},
		Status: openchoreov1alpha1.ReleaseStatus{
			Resources: []openchoreov1alpha1.ResourceStatus{
				{HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
			},
		},
	}
	comp := &openchoreov1alpha1.Component{
		Spec: openchoreov1alpha1.ComponentSpec{
			ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "unknown/something"},
		},
	}

	err := r.setResourcesReadyStatus(testContext(), rb, release, comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cond := findCondition(rb.Status.Conditions, string(ConditionResourcesReady))
	if cond == nil {
		t.Fatal("ResourcesReady condition should be set")
	}
	// Generic evaluation: single healthy → ready
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("Status = %q, want True", cond.Status)
	}
}

// ─── evaluateStatefulSetStatus ───────────────────────────────────────────────

func statefulSetResource(status openchoreov1alpha1.HealthStatus) openchoreov1alpha1.ResourceStatus {
	return openchoreov1alpha1.ResourceStatus{
		Group:        "apps",
		Version:      "v1",
		Kind:         "StatefulSet",
		Name:         "test-sts",
		HealthStatus: status,
	}
}

func TestEvaluateStatefulSetStatus_AllPathsAndNoPrimary(t *testing.T) {
	tests := []struct {
		status     openchoreov1alpha1.HealthStatus
		wantReady  bool
		wantReason string
	}{
		{openchoreov1alpha1.HealthStatusDegraded, false, string(ReasonResourcesDegraded)},
		{openchoreov1alpha1.HealthStatusProgressing, false, string(ReasonResourcesProgressing)},
		{openchoreov1alpha1.HealthStatusUnknown, false, string(ReasonResourcesUnknown)},
		{openchoreov1alpha1.HealthStatusSuspended, true, string(ReasonReadyWithSuspendedResources)},
		{openchoreov1alpha1.HealthStatusHealthy, true, string(ReasonReady)},
	}
	for _, tc := range tests {
		ready, reason, _ := evaluateStatefulSetStatus(
			[]openchoreov1alpha1.ResourceStatus{statefulSetResource(tc.status)},
			WorkloadTypeStatefulSet,
		)
		if ready != tc.wantReady {
			t.Errorf("status=%q: ready=%v, want %v", tc.status, ready, tc.wantReady)
		}
		if reason != tc.wantReason {
			t.Errorf("status=%q: reason=%q, want %q", tc.status, reason, tc.wantReason)
		}
	}

	// No primary StatefulSet
	ready, reason, _ := evaluateStatefulSetStatus([]openchoreov1alpha1.ResourceStatus{}, WorkloadTypeStatefulSet)
	if ready || reason != string(ReasonResourcesProgressing) {
		t.Errorf("no primary: ready=%v reason=%q, want false/%q", ready, reason, ReasonResourcesProgressing)
	}
}

func TestEvaluateStatefulSetStatus_PrimaryHealthy_SupportingDegraded(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		statefulSetResource(openchoreov1alpha1.HealthStatusHealthy),
		serviceResource(openchoreov1alpha1.HealthStatusDegraded),
	}
	ready, reason, _ := evaluateStatefulSetStatus(resources, WorkloadTypeStatefulSet)
	if ready {
		t.Error("should not be ready when supporting resource is degraded")
	}
	if reason != string(ReasonResourcesDegraded) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesDegraded)
	}
}

func TestEvaluateStatefulSetStatus_PrimaryHealthy_AllHealthy(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		statefulSetResource(openchoreov1alpha1.HealthStatusHealthy),
		serviceResource(openchoreov1alpha1.HealthStatusHealthy),
	}
	ready, reason, _ := evaluateStatefulSetStatus(resources, WorkloadTypeStatefulSet)
	if !ready {
		t.Error("should be ready when primary and all supporting resources are healthy")
	}
	if reason != string(ReasonReady) {
		t.Errorf("reason = %q, want %q", reason, ReasonReady)
	}
}

func TestEvaluateStatefulSetStatus_PrimaryHealthy_OperationalDegraded(t *testing.T) {
	resources := []openchoreov1alpha1.ResourceStatus{
		statefulSetResource(openchoreov1alpha1.HealthStatusHealthy),
		hpaResource(openchoreov1alpha1.HealthStatusDegraded),
	}
	ready, reason, _ := evaluateStatefulSetStatus(resources, WorkloadTypeStatefulSet)
	if ready {
		t.Error("should not be ready when operational resource is degraded")
	}
	if reason != string(ReasonResourcesDegraded) {
		t.Errorf("reason = %q, want %q", reason, ReasonResourcesDegraded)
	}
}

// ─── buildMetadataContext label content (improved) ───────────────────────────

func TestBuildMetadataContext_LabelContent(t *testing.T) {
	r := newTestReconciler()

	namespaceName := "test-ns"
	projectName := "test-proj"
	componentName := "test-comp"
	environmentName := "test-env"
	componentUID := "comp-uid-aaa"
	projectUID := "proj-uid-bbb"
	environmentUID := "env-uid-ccc"
	dataPlaneName := "test-dp"

	componentRelease := &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
		},
	}
	component := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{UID: "comp-uid-aaa"},
	}
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{UID: "proj-uid-bbb"},
	}
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: dataPlaneName, UID: "dp-uid-ddd"},
	}
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{UID: "env-uid-ccc"},
	}

	mctx := r.buildMetadataContext(componentRelease, component, project, dataPlane, environment, environmentName)

	// Verify Labels contain specific key-value pairs
	expectedLabels := map[string]string{
		labels.LabelKeyNamespaceName:   namespaceName,
		labels.LabelKeyProjectName:     projectName,
		labels.LabelKeyComponentName:   componentName,
		labels.LabelKeyEnvironmentName: environmentName,
		labels.LabelKeyComponentUID:    componentUID,
		labels.LabelKeyProjectUID:      projectUID,
		labels.LabelKeyEnvironmentUID:  environmentUID,
	}
	for k, v := range expectedLabels {
		got, ok := mctx.Labels[k]
		if !ok {
			t.Errorf("Labels missing key %q", k)
		} else if got != v {
			t.Errorf("Labels[%q] = %q, want %q", k, got, v)
		}
	}

	if len(mctx.Labels) != len(expectedLabels) {
		t.Errorf("Labels has %d entries, want %d", len(mctx.Labels), len(expectedLabels))
	}

	// Verify PodSelectors contain the same specific key-value pairs
	for k, v := range expectedLabels {
		got, ok := mctx.PodSelectors[k]
		if !ok {
			t.Errorf("PodSelectors missing key %q", k)
		} else if got != v {
			t.Errorf("PodSelectors[%q] = %q, want %q", k, got, v)
		}
	}

	if len(mctx.PodSelectors) != len(expectedLabels) {
		t.Errorf("PodSelectors has %d entries, want %d", len(mctx.PodSelectors), len(expectedLabels))
	}

	// Verify Annotations is non-nil but empty
	if mctx.Annotations == nil {
		t.Error("Annotations should not be nil")
	}
	if len(mctx.Annotations) != 0 {
		t.Errorf("Annotations should be empty, got %d entries", len(mctx.Annotations))
	}

	// Verify ComponentNamespace is set correctly
	if mctx.ComponentNamespace != namespaceName {
		t.Errorf("ComponentNamespace = %q, want %q", mctx.ComponentNamespace, namespaceName)
	}
}

// findCondition is a test helper that looks up a condition by type.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
