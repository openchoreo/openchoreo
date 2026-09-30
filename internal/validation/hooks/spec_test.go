// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func strPtr(s string) *string { return &s }

func clusterRef() *v1alpha1.WorkflowRef {
	return &v1alpha1.WorkflowRef{Kind: v1alpha1.WorkflowRefKindClusterWorkflow, Name: "scan"}
}

func TestValidateHookSpec(t *testing.T) {
	tests := []struct {
		name      string
		spec      *v1alpha1.HookSpec
		isCluster bool
		wantErrs  []string // substrings of the expected errors; empty means accept
	}{
		{
			name: "accepts a workflow hook with every parameter source",
			spec: &v1alpha1.HookSpec{Type: v1alpha1.HookTypeWorkflow, WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "severity", Value: strPtr("HIGH")},
				{Name: "image", From: "${deployment.workload.containers.main.image}"},
				{Name: "threshold", From: "${deployment.environment.name}", Overridable: true},
				{Name: "timeout", Default: strPtr("10m")},
				{Name: "ticket", Required: true},
				{Name: "mode", Default: strPtr("fast"), Schema: &runtime.RawExtension{Raw: []byte(`{"type":"string","enum":["fast","slow"]}`)}},
			}},
		},
		{
			name: "accepts an empty type because the CRD defaults it to Workflow",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef()},
		},
		{
			name: "accepts a namespaced Workflow ref on a namespaced Hook",
			spec: &v1alpha1.HookSpec{WorkflowRef: &v1alpha1.WorkflowRef{Kind: v1alpha1.WorkflowRefKindWorkflow, Name: "scan"}},
		},
		{
			name:     "rejects an unsupported executor type since only Workflow is implemented",
			spec:     &v1alpha1.HookSpec{Type: "Job", WorkflowRef: clusterRef()},
			wantErrs: []string{"spec.type"},
		},
		{
			name:     "rejects a missing workflowRef since a Workflow hook has nothing to run",
			spec:     &v1alpha1.HookSpec{Type: v1alpha1.HookTypeWorkflow},
			wantErrs: []string{"spec.workflowRef: Required"},
		},
		{
			name:     "rejects an empty workflow name",
			spec:     &v1alpha1.HookSpec{WorkflowRef: &v1alpha1.WorkflowRef{Kind: v1alpha1.WorkflowRefKindClusterWorkflow}},
			wantErrs: []string{"spec.workflowRef.name: Required"},
		},
		{
			name:      "rejects a namespaced Workflow ref on a ClusterHook because it has no namespace to resolve in",
			spec:      &v1alpha1.HookSpec{WorkflowRef: &v1alpha1.WorkflowRef{Kind: v1alpha1.WorkflowRefKindWorkflow, Name: "scan"}},
			isCluster: true,
			wantErrs:  []string{"spec.workflowRef.kind", "ClusterHook may only reference a ClusterWorkflow"},
		},
		{
			name:     "rejects an unknown workflowRef kind",
			spec:     &v1alpha1.HookSpec{WorkflowRef: &v1alpha1.WorkflowRef{Kind: "Template", Name: "scan"}},
			wantErrs: []string{"spec.workflowRef.kind"},
		},
		{
			name:     "rejects a parameter with no source because the input would be unset",
			spec:     &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{{Name: "image"}}},
			wantErrs: []string{"spec.parameters[0]: Required", "exactly one of value, from, default or required"},
		},
		{
			name: "rejects two sources because precedence would be ambiguous",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "image", Value: strPtr("x"), Default: strPtr("y")}}},
			wantErrs: []string{"got value and default"},
		},
		{
			name: "rejects required combined with from",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "image", From: "${deployment.release}", Required: true}}},
			wantErrs: []string{"got from and required"},
		},
		{
			name: "rejects overridable without from since a fixed value is never overridable",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "severity", Value: strPtr("HIGH"), Overridable: true}}},
			wantErrs: []string{"spec.parameters[0].overridable", "only meaningful together with from"},
		},
		{
			name: "rejects a from without a ${...} marker since it would be passed as a literal",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "image", From: "deployment.release"}}},
			wantErrs: []string{"spec.parameters[0].from", "must contain a ${...} CEL expression"},
		},
		{
			name: "rejects a from that does not compile",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "image", From: "${deployment.release +}"}}},
			wantErrs: []string{"spec.parameters[0].from", "CEL compilation failed"},
		},
		{
			name: "rejects a from that references an unknown root variable",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "image", From: "${release.name}"}}},
			wantErrs: []string{"CEL compilation failed", "undeclared reference"},
		},
		{
			name: "accepts interpolation with several expressions",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "tag", From: "${deployment.project.name}-${deployment.component.name}"}}},
		},
		{
			name: "rejects duplicate parameter names since the list is keyed by name",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "image", Required: true}, {Name: "image", Required: true}}},
			wantErrs: []string{"spec.parameters[1].name: Duplicate"},
		},
		{
			name: "rejects a schema fragment with an unknown key",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "mode", Required: true, Schema: &runtime.RawExtension{Raw: []byte(`{"types":"string"}`)}}}},
			wantErrs: []string{"spec.parameters[0].schema", "unknown or invalid fields"},
		},
		{
			name: "rejects a schema fragment that is not an object",
			spec: &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
				{Name: "mode", Required: true, Schema: &runtime.RawExtension{Raw: []byte(`"string"`)}}}},
			wantErrs: []string{"schema must be a JSON object"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateHookSpec(tc.spec, tc.isCluster, field.NewPath("spec"))
			assertErrs(t, errs, tc.wantErrs)
		})
	}
}

// assertErrs fails when accept/reject does not match, or when an expected substring
// is missing from the joined error text.
func assertErrs(t *testing.T, errs field.ErrorList, want []string) {
	t.Helper()
	if len(want) == 0 {
		if len(errs) > 0 {
			t.Fatalf("expected no errors, got: %v", errs)
		}
		return
	}
	if len(errs) == 0 {
		t.Fatalf("expected errors containing %v, got none", want)
	}
	joined := errs.ToAggregate().Error()
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("expected error containing %q, got: %s", w, joined)
		}
	}
}

// enabledTo is validated at admission because a typo'd kind or an empty name
// would silently disable the hook everywhere (EnabledFor never matches).
func TestValidateHookSpecEnabledTo(t *testing.T) {
	cases := []struct {
		name     string
		refs     []v1alpha1.HookSubjectRef
		wantErrs []string
	}{
		{name: "accepts both kinds", refs: []v1alpha1.HookSubjectRef{
			{Kind: v1alpha1.HookSubjectRefKindComponentType, Name: "svc"},
			{Kind: v1alpha1.HookSubjectRefKindClusterComponentType, Name: "service"}}},
		{name: "rejects a project-type kind since enabledTo scopes component types only",
			refs:     []v1alpha1.HookSubjectRef{{Kind: "ClusterProjectType", Name: "default"}},
			wantErrs: []string{"spec.enabledTo[0].kind: Unsupported"}},
		{name: "rejects an empty name",
			refs:     []v1alpha1.HookSubjectRef{{Kind: v1alpha1.HookSubjectRefKindComponentType}},
			wantErrs: []string{"spec.enabledTo[0].name: Required"}},
		{name: "rejects a duplicate entry",
			refs: []v1alpha1.HookSubjectRef{
				{Kind: v1alpha1.HookSubjectRefKindClusterComponentType, Name: "service"},
				{Kind: v1alpha1.HookSubjectRefKindClusterComponentType, Name: "service"}},
			wantErrs: []string{"spec.enabledTo[1]: Duplicate"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := &v1alpha1.HookSpec{WorkflowRef: clusterRef(), EnabledTo: tc.refs}
			errs := ValidateHookSpec(spec, true, field.NewPath("spec"))
			if len(tc.wantErrs) == 0 {
				if len(errs) != 0 {
					t.Fatalf("expected accept, got %v", errs)
				}
				return
			}
			got := errs.ToAggregate().Error()
			for _, w := range tc.wantErrs {
				if !strings.Contains(got, w) {
					t.Fatalf("expected error containing %q, got %q", w, got)
				}
			}
		})
	}
}
