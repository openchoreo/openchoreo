// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func raw(s string) *runtime.RawExtension { return &runtime.RawExtension{Raw: []byte(s)} }

// scanHook declares one parameter of every openness: fixed, computed, computed but
// overridable, defaulted and required.
var scanHook = &v1alpha1.HookSpec{WorkflowRef: clusterRef(), Parameters: []v1alpha1.HookParameter{
	{Name: "severity", Value: strPtr("HIGH")},
	{Name: "image", From: "${deployment.workload.containers.main.image}"},
	{Name: "threshold", From: "${deployment.environment.name}", Overridable: true},
	{Name: "timeout", Default: strPtr("10m")},
	{Name: "ticket", Required: true},
}}

func resolveScan(ref v1alpha1.HookRef) (*v1alpha1.HookSpec, error) {
	switch ref.Name {
	case "scan":
		return scanHook, nil
	case "broken":
		return nil, errors.New("api unavailable")
	}
	return nil, nil
}

func binding(name string, extra ...func(*v1alpha1.HookBinding)) v1alpha1.HookBinding {
	b := v1alpha1.HookBinding{Name: name, HookRef: v1alpha1.HookRef{Kind: v1alpha1.HookRefKindClusterHook, Name: "scan"},
		Parameters: raw(`{"ticket":"OPS-1"}`)}
	for _, f := range extra {
		f(&b)
	}
	return b
}

func TestValidateHookSet(t *testing.T) {
	tests := []struct {
		name         string
		set          *v1alpha1.HookSet
		resolve      HookResolver
		wantErrs     []string
		wantWarnings []string
	}{
		{name: "nil set is accepted so existing pipelines are untouched"},
		{
			name: "accepts a Sync pre-deploy Block binding with timeout, retries, appliesTo and triggers",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("image-scan", func(b *v1alpha1.HookBinding) {
				b.Mode = v1alpha1.HookModeSync
				b.OnFailure = v1alpha1.HookFailurePolicyBlock
				b.Timeout = "20m"
				b.Retries = 2
				b.AppliesTo = []v1alpha1.HookSubjectSelector{{Kind: v1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "service"}}
				b.Parameters = raw(`{"ticket":"OPS-1","threshold":"3","timeout":"5m"}`)
			})}},
			resolve: resolveScan,
		},
		{
			name: "accepts an Async post-deploy binding with nothing but a hookRef",
			set: &v1alpha1.HookSet{PostDeploy: []v1alpha1.HookBinding{binding("notify", func(b *v1alpha1.HookBinding) {
				b.Mode = v1alpha1.HookModeAsync
			})}},
			resolve: resolveScan,
		},
		{
			name: "rejects the same name in pre and post since status and retry are keyed by name",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan")},
				PostDeploy: []v1alpha1.HookBinding{binding("scan")}},
			wantErrs: []string{"hooks.postDeploy[0].name: Duplicate", "already used at hooks.preDeploy[0]"},
		},
		{
			name:     "rejects a missing binding name",
			set:      &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("")}},
			wantErrs: []string{"hooks.preDeploy[0].name: Required"},
		},
		{
			name: "rejects a missing hookRef name",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.HookRef.Name = ""
			})}},
			wantErrs: []string{"hooks.preDeploy[0].hookRef.name: Required"},
		},
		{
			name: "rejects an unknown hookRef kind",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.HookRef.Kind = "Trait"
			})}},
			wantErrs: []string{"hooks.preDeploy[0].hookRef.kind: Unsupported"},
		},
		{
			name: "rejects onFailure, timeout and retries on Async since they could never take effect",
			set: &v1alpha1.HookSet{PostDeploy: []v1alpha1.HookBinding{binding("notify", func(b *v1alpha1.HookBinding) {
				b.Mode = v1alpha1.HookModeAsync
				b.OnFailure = v1alpha1.HookFailurePolicyIgnore
				b.Timeout = "5m"
				b.Retries = 1
			})}},
			wantErrs: []string{"postDeploy[0].onFailure: Forbidden", "postDeploy[0].timeout: Forbidden", "postDeploy[0].retries: Forbidden"},
		},
		{
			name: "rejects an unknown mode",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Mode = "Detached"
			})}},
			wantErrs: []string{"preDeploy[0].mode: Unsupported"},
		},
		{
			name: "rejects Block on post-deploy since the release is already rendered",
			set: &v1alpha1.HookSet{PostDeploy: []v1alpha1.HookBinding{binding("smoke", func(b *v1alpha1.HookBinding) {
				b.OnFailure = v1alpha1.HookFailurePolicyBlock
			})}},
			wantErrs: []string{"postDeploy[0].onFailure", "Block is only valid for preDeploy"},
		},
		{
			name: "rejects Alert on pre-deploy since there is nothing running to degrade",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.OnFailure = v1alpha1.HookFailurePolicyAlert
			})}},
			wantErrs: []string{"preDeploy[0].onFailure", "Alert is only valid for postDeploy"},
		},
		{
			name: "accepts Alert on post-deploy and Ignore on pre-deploy",
			set: &v1alpha1.HookSet{
				PreDeploy:  []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) { b.OnFailure = v1alpha1.HookFailurePolicyIgnore })},
				PostDeploy: []v1alpha1.HookBinding{binding("smoke", func(b *v1alpha1.HookBinding) { b.OnFailure = v1alpha1.HookFailurePolicyAlert })},
			},
		},
		{
			name: "rejects an unknown onFailure",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.OnFailure = "Rollback"
			})}},
			wantErrs: []string{"preDeploy[0].onFailure: Unsupported"},
		},
		{
			name: "rejects an unknown appliesTo kind and a missing name",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.AppliesTo = []v1alpha1.HookSubjectSelector{{Kind: "Component", Name: "x"}, {Kind: v1alpha1.HookSubjectSelectorKindComponentType}}
			})}},
			wantErrs: []string{"appliesTo[0].kind: Unsupported", "appliesTo[1].name: Required"},
		},
		{
			name: "rejects a timeout that is not a duration",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Timeout = "20minutes"
			})}},
			wantErrs: []string{"preDeploy[0].timeout", "Go duration"},
		},
		{
			name: "rejects a zero timeout since the hook could never finish in time",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Timeout = "0s"
			})}},
			wantErrs: []string{"timeout must be positive"},
		},
		{
			name: "rejects retries above 5",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Retries = 6
			})}},
			wantErrs: []string{"preDeploy[0].retries", "between 0 and 5"},
		},
		{
			name: "rejects parameters that are not a string map",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Parameters = raw(`{"threshold":3}`)
			})}},
			resolve:  resolveScan,
			wantErrs: []string{"preDeploy[0].parameters", "object of string values"},
		},
		{
			name: "rejects a key the hook does not declare",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Parameters = raw(`{"ticket":"OPS-1","shade":"red"}`)
			})}},
			resolve:  resolveScan,
			wantErrs: []string{`parameters[shade]`, "not declared by the hook"},
		},
		{
			name: "rejects overriding a fixed value",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Parameters = raw(`{"ticket":"OPS-1","severity":"LOW"}`)
			})}},
			resolve:  resolveScan,
			wantErrs: []string{`parameters[severity]: Forbidden`, "fixed by the hook"},
		},
		{
			name: "rejects overriding a computed value that is not overridable",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Parameters = raw(`{"ticket":"OPS-1","image":"nginx"}`)
			})}},
			resolve:  resolveScan,
			wantErrs: []string{`parameters[image]: Forbidden`, "not overridable"},
		},
		{
			name: "rejects a binding that omits a required parameter",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.Parameters = nil
			})}},
			resolve:  resolveScan,
			wantErrs: []string{`parameters[ticket]: Required`},
		},
		{
			name: "warns instead of rejecting when the hook does not exist yet",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.HookRef.Name = "later"
			})}},
			resolve:      resolveScan,
			wantWarnings: []string{`ClusterHook "later" not found`},
		},
		{
			name: "warns when the hook cannot be resolved",
			set: &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) {
				b.HookRef.Name = "broken"
			})}},
			resolve:      resolveScan,
			wantWarnings: []string{`could not resolve ClusterHook "broken"`, "api unavailable"},
		},
		{
			name:    "does not check parameters without a resolver",
			set:     &v1alpha1.HookSet{PreDeploy: []v1alpha1.HookBinding{binding("scan", func(b *v1alpha1.HookBinding) { b.Parameters = raw(`{"shade":"red"}`) })}},
			resolve: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs, warnings := ValidateHookSet(tc.set, field.NewPath("hooks"), tc.resolve)
			assertErrs(t, errs, tc.wantErrs)
			joined := strings.Join(warnings, "\n")
			for _, w := range tc.wantWarnings {
				if !strings.Contains(joined, w) {
					t.Errorf("expected warning containing %q, got: %q", w, joined)
				}
			}
			if len(tc.wantWarnings) == 0 && len(warnings) > 0 {
				t.Errorf("expected no warnings, got: %q", warnings)
			}
		})
	}
}

// A binding whose appliesTo names a component type the hook is not enabled for
// would be Skipped for every component; that is a pipeline mistake and must be
// rejected at admission, while project-type selectors are left alone.
func TestValidateAppliesToAgainstEnabledTo(t *testing.T) {
	enabled := []v1alpha1.HookSubjectRef{{Kind: v1alpha1.HookSubjectRefKindClusterComponentType, Name: "service"}}
	cases := []struct {
		name    string
		applies []v1alpha1.HookSubjectSelector
		enabled []v1alpha1.HookSubjectRef
		wantErr bool
	}{
		{name: "no enabledTo accepts any appliesTo", applies: []v1alpha1.HookSubjectSelector{{Kind: v1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "worker"}}},
		{name: "empty appliesTo is fine (hook narrows on its own)", enabled: enabled},
		{name: "matching selector accepted", enabled: enabled,
			applies: []v1alpha1.HookSubjectSelector{{Kind: v1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "service"}}},
		{name: "selector outside enabledTo rejected", enabled: enabled, wantErr: true,
			applies: []v1alpha1.HookSubjectSelector{{Kind: v1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "worker"}}},
		{name: "same name but namespaced kind rejected", enabled: enabled, wantErr: true,
			applies: []v1alpha1.HookSubjectSelector{{Kind: v1alpha1.HookSubjectSelectorKindComponentType, Name: "service"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateAppliesToAgainstEnabledTo(tc.applies, tc.enabled, field.NewPath("appliesTo"))
			if tc.wantErr && len(errs) == 0 {
				t.Fatal("expected an error")
			}
			if !tc.wantErr && len(errs) != 0 {
				t.Fatalf("expected accept, got %v", errs)
			}
		})
	}
}
