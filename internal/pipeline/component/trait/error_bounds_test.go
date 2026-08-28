// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// The engine bounds every expression it names in an error, but a trait patch or remove
// that fails is re-wrapped by this package - and a wrap that quotes the raw expression
// puts the whole tenant-authored fragment back into a message that ends up verbatim in a
// status condition. An oversized condition is rejected by the API server, and a failed
// status write is exactly the hot-backoff failure mode the cost guards exist to avoid.
//
// maxWrappedErrorLen is a generous ceiling: every message here names a handful of bounded
// fragments, so a message anywhere near this size means a raw expression leaked through.
const maxWrappedErrorLen = 4096

// hugeExpr builds a syntactically valid expression that fails at evaluation time (index
// out of range) and carries a padded string literal, so a wrap that quotes it raw is
// immediately visible in the message length. A runtime failure is used rather than a
// compile error because cel-go's own compile diagnostics quote the source line.
func hugeExpr() string {
	return `${["` + strings.Repeat("a", 50_000) + `"][7]}`
}

// traitWithPatch builds a trait carrying one patch, so each case can vary just the
// expression under test.
func traitWithPatch(patch v1alpha1.TraitPatch) *v1alpha1.Trait {
	return &v1alpha1.Trait{Spec: v1alpha1.TraitSpec{Patches: []v1alpha1.TraitPatch{patch}}}
}

func traitWithRemove(remove v1alpha1.TraitRemove) *v1alpha1.Trait {
	return &v1alpha1.Trait{Spec: v1alpha1.TraitSpec{Removes: []v1alpha1.TraitRemove{remove}}}
}

func TestTraitErrorsStayBounded(t *testing.T) {
	patchOps := []v1alpha1.JSONPatchOperation{{Op: "add", Path: "/data/x", Value: &runtime.RawExtension{Raw: []byte(`"v"`)}}}

	tests := []struct {
		name string
		run  func(p *Processor) error
	}{
		{
			name: "patch forEach evaluation failure",
			run: func(p *Processor) error {
				return p.ApplyTraitPatches(t.Context(), buildTargets(1), traitWithPatch(v1alpha1.TraitPatch{
					Target:     v1alpha1.PatchTarget{Kind: "ConfigMap"},
					ForEach:    hugeExpr(),
					Operations: patchOps,
				}), map[string]any{})
			},
		},
		{
			name: "patch where evaluation failure",
			run: func(p *Processor) error {
				return p.ApplyTraitPatches(t.Context(), buildTargets(1), traitWithPatch(v1alpha1.TraitPatch{
					Target:     v1alpha1.PatchTarget{Kind: "ConfigMap", Where: hugeExpr()},
					Operations: patchOps,
				}), map[string]any{})
			},
		},
		{
			name: "remove forEach evaluation failure",
			run: func(p *Processor) error {
				_, err := p.ApplyTraitRemoves(t.Context(), buildTargets(1), traitWithRemove(v1alpha1.TraitRemove{
					Target:  v1alpha1.PatchTarget{Kind: "ConfigMap"},
					ForEach: hugeExpr(),
				}), map[string]any{})
				return err
			},
		},
		{
			name: "remove where evaluation failure",
			run: func(p *Processor) error {
				_, err := p.ApplyTraitRemoves(t.Context(), buildTargets(1), traitWithRemove(v1alpha1.TraitRemove{
					Target: v1alpha1.PatchTarget{Kind: "ConfigMap", Where: hugeExpr()},
				}), map[string]any{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(NewProcessor(template.NewEngine()))
			if err == nil {
				t.Fatal("expected the expression to fail, got nil")
			}
			if got := len(err.Error()); got > maxWrappedErrorLen {
				t.Fatalf("error message is %d bytes, want at most %d - a raw expression leaked into the wrap: %.500s",
					got, maxWrappedErrorLen, err.Error())
			}
		})
	}
}
