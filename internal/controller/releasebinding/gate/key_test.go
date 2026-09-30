// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// The key format is persisted on ReleaseBindings, so it is pinned to its exact
// serialization: an environment without hooks hashes the empty list, and a
// zero sequence is omitted so keys written before the field existed still match.
func TestKeyFormatForEmptySet(t *testing.T) {
	key, hash, err := Key("rel-1", 0, EffectiveHookSet{})
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256Hex(`[]`)
	if hash != wantHash {
		t.Fatalf("hookSetHash = %s, want %s", hash, wantHash)
	}
	if want := sha256Hex(`{"hookSetHash":"` + wantHash + `","release":"rel-1"}`); key != want {
		t.Fatalf("key = %s, want %s", key, want)
	}

	keySeq, _, err := Key("rel-1", 2, EffectiveHookSet{})
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256Hex(`{"hookSetHash":"` + wantHash + `","release":"rel-1","sequence":2}`); keySeq != want {
		t.Fatalf("key with sequence = %s, want %s", keySeq, want)
	}
}

func resolvedScan() ResolvedHook {
	return ResolvedHook{
		Phase: PhasePreDeploy,
		Binding: openchoreov1alpha1.HookBinding{
			Name:       "scan",
			HookRef:    openchoreov1alpha1.HookRef{Name: "trivy"},
			Parameters: &runtime.RawExtension{Raw: []byte(`{"severity":"HIGH","threshold":5}`)},
		},
		Hook: &openchoreov1alpha1.HookSpec{
			Type:       openchoreov1alpha1.HookTypeWorkflow,
			Parameters: []openchoreov1alpha1.HookParameter{{Name: "severity"}, {Name: "threshold"}},
		},
	}
}

// A new key starts a new attempt and re-runs every hook, so the hash must move
// exactly when what would run changes, and stay put for differences that do not
// change what runs (a key that moves on noise re-runs approvals and scans for
// nothing; one that misses a real change reuses stale results).
func TestKeyHookSetHashSensitivity(t *testing.T) {
	base := func() EffectiveHookSet { return EffectiveHookSet{PreDeploy: []ResolvedHook{resolvedScan()}} }
	_, baseHash, err := Key("rel", 0, base())
	if err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		mutate  func(*EffectiveHookSet)
		changes bool
	}{
		"parameter key order": {changes: false, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Binding.Parameters.Raw = []byte(`{"threshold":5,"severity":"HIGH"}`)
		}},
		"parameter whitespace": {changes: false, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Binding.Parameters.Raw = []byte("{ \"severity\" : \"HIGH\",\n \"threshold\": 5 }")
		}},
		"skip reason is not part of the key": {changes: false, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].SkipReason = SkipReasonNotApplicable
		}},
		"parameter value": {changes: true, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Binding.Parameters.Raw = []byte(`{"severity":"LOW","threshold":5}`)
		}},
		"hook spec": {changes: true, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Hook.Parameters[0].Default = strp("CRITICAL")
		}},
		"hook ref": {changes: true, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Binding.HookRef.Kind = openchoreov1alpha1.HookRefKindClusterHook
		}},
		"mode": {changes: true, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Binding.Mode = openchoreov1alpha1.HookModeAsync
		}},
		// A hook created after the binding referenced it must start a new
		// attempt, or the gate would stay blocked on the NotFound result.
		"hook not found": {changes: true, mutate: func(s *EffectiveHookSet) {
			s.PreDeploy[0].Hook, s.PreDeploy[0].NotFound = nil, true
		}},
		"phase": {changes: true, mutate: func(s *EffectiveHookSet) {
			h := s.PreDeploy[0]
			h.Phase = PhasePostDeploy
			s.PreDeploy, s.PostDeploy = nil, []ResolvedHook{h}
		}},
		"extra binding": {changes: true, mutate: func(s *EffectiveHookSet) {
			h := resolvedScan()
			h.Binding.Name = "scan-2"
			s.PreDeploy = append(s.PreDeploy, h)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			set := base()
			tc.mutate(&set)
			_, hash, err := Key("rel", 0, set)
			if err != nil {
				t.Fatal(err)
			}
			if changed := hash != baseHash; changed != tc.changes {
				t.Fatalf("hash changed = %v, want %v", changed, tc.changes)
			}
		})
	}
}

// Environments list bindings in whatever order authors wrote them; reordering
// them in the spec must not start a new attempt. Resolve sorts by name, so the
// key computed from its output is order-independent.
func TestKeyIndependentOfBindingOrder(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(hookObj("approve"), hookObj("notify"), clusterHookObj("trivy")).Build()
	scan := binding("scan", openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "trivy"})
	approval := binding("approval", openchoreov1alpha1.HookRef{Name: "approve"})
	notice := binding("notice", openchoreov1alpha1.HookRef{Name: "notify"})

	keyOf := func(pre ...openchoreov1alpha1.HookBinding) string {
		t.Helper()
		set, err := Resolve(context.Background(), c, environmentWith("prod", &openchoreov1alpha1.HookSet{PreDeploy: pre}), Subject{})
		if err != nil {
			t.Fatal(err)
		}
		key, _, err := Key("rel", 0, set)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	k1 := keyOf(scan, approval, notice)
	if k2 := keyOf(notice, scan, approval); k2 != k1 {
		t.Fatalf("reordered bindings changed the key: %s vs %s", k1, k2)
	}
	if k3 := keyOf(scan, approval); k3 == k1 {
		t.Fatal("dropping a binding must change the key")
	}
}

// A binding whose parameters are not valid JSON cannot be hashed; the key must
// fail with the binding named rather than hash a truncated representation.
func TestKeyRejectsInvalidBindingParameters(t *testing.T) {
	h := resolvedScan()
	h.Binding.Parameters.Raw = []byte(`{not json`)
	_, _, err := Key("rel", 0, EffectiveHookSet{PostDeploy: []ResolvedHook{h}})
	if err == nil || !strings.Contains(err.Error(), `binding "scan"`) {
		t.Fatalf("got %v, want an error naming binding \"scan\"", err)
	}
}
