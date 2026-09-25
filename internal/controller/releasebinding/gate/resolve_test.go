// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var errAPIDown = errors.New("apiserver unavailable")

// failingGets returns a client whose every Get fails with errAPIDown.
func failingGets(t *testing.T) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(newScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errAPIDown
		},
	}).Build()
}

// A transient API error is not "the hook is missing": treating it as NotFound
// would block (or skip) a deployment on a flake. Resolve must return the error,
// naming the binding, in either phase, so the reconcile is retried.
func TestResolvePropagatesAPIErrors(t *testing.T) {
	ref := openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "trivy"}
	cases := map[string]*openchoreov1alpha1.HookSet{
		"preDeploy":  {PreDeploy: []openchoreov1alpha1.HookBinding{binding("scan", ref)}},
		"postDeploy": {PostDeploy: []openchoreov1alpha1.HookBinding{binding("scan", openchoreov1alpha1.HookRef{Name: "smoke"})}},
	}
	for phase, hooks := range cases {
		t.Run(phase, func(t *testing.T) {
			_, err := Resolve(context.Background(), failingGets(t), environmentWith("prod", hooks), Subject{})
			if !errors.Is(err, errAPIDown) {
				t.Fatalf("err = %v, want it to wrap %v", err, errAPIDown)
			}
			if want := `resolving hook for binding "scan": apiserver unavailable`; err.Error() != want {
				t.Fatalf("err = %q, want %q", err.Error(), want)
			}
		})
	}
}

// A ClusterHook that does not exist yet is recorded on the binding (so the gate
// can report HookNotFound on exactly that binding) rather than failing the whole
// set. With no spec there is no enabledTo to evaluate, so it is not Skipped.
func TestResolveMissingClusterHook(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	env := environmentWith("prod", &openchoreov1alpha1.HookSet{
		PreDeploy: []openchoreov1alpha1.HookBinding{binding("scan",
			openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "not-yet"})},
	})
	set, err := Resolve(context.Background(), c, env, Subject{})
	if err != nil {
		t.Fatalf("a missing hook must not be an error, got %v", err)
	}
	if len(set.PreDeploy) != 1 {
		t.Fatalf("got %d bindings, want 1", len(set.PreDeploy))
	}
	got := set.PreDeploy[0]
	if !got.NotFound || got.Hook != nil || got.SkipReason != "" || got.Phase != PhasePreDeploy {
		t.Fatalf("got %+v, want NotFound with no hook, no skip reason, phase preDeploy", got)
	}
}

// A Hook ref with an empty kind means a namespaced Hook in the environment's
// namespace, never a ClusterHook of the same name: picking the wrong one would
// run a different workflow than the platform engineer bound.
func TestResolveEmptyKindMeansNamespacedHook(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(clusterHookObj("smoke")).Build()
	env := environmentWith("prod", &openchoreov1alpha1.HookSet{
		PostDeploy: []openchoreov1alpha1.HookBinding{binding("smoke", openchoreov1alpha1.HookRef{Name: "smoke"})},
	})
	set, err := Resolve(context.Background(), c, env, Subject{})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.PostDeploy) != 1 || !set.PostDeploy[0].NotFound || set.PostDeploy[0].Phase != PhasePostDeploy {
		t.Fatalf("an empty kind must not resolve to the ClusterHook, got %+v", set.PostDeploy)
	}
}

// An absent or empty HookSet means no gate at all; resolving it must not touch
// the API (every Get here would fail) and must yield an empty set.
func TestResolveEmptyHookSet(t *testing.T) {
	for name, env := range map[string]*openchoreov1alpha1.Environment{
		"nil environment": nil,
		"nil hooks":       environmentWith("prod", nil),
		"empty hooks":     environmentWith("prod", &openchoreov1alpha1.HookSet{}),
	} {
		t.Run(name, func(t *testing.T) {
			set, err := Resolve(context.Background(), failingGets(t), env, Subject{})
			if err != nil {
				t.Fatal(err)
			}
			if !set.Empty() {
				t.Fatalf("got %+v, want an empty set", set)
			}
		})
	}
}

// appliesTo and enabledTo distinguish a namespaced ComponentType from a
// ClusterComponentType of the same name; conflating them would run a hook on
// components the platform engineer never targeted.
func TestSubjectKindMatching(t *testing.T) {
	nsSvc := Subject{ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindComponentType, ComponentTypeName: "deployment/service"}
	clusterSvc := Subject{ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType, ComponentTypeName: "deployment/service"}

	clusterSel := []openchoreov1alpha1.HookSubjectSelector{{Kind: openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, Name: "service"}}
	if !Matches(clusterSel, clusterSvc) {
		t.Fatal("a ClusterComponentType selector must match a ClusterComponentType subject")
	}
	if Matches(clusterSel, nsSvc) {
		t.Fatal("a ClusterComponentType selector must not match a namespaced ComponentType subject")
	}
	nsSel := []openchoreov1alpha1.HookSubjectSelector{{Kind: openchoreov1alpha1.HookSubjectSelectorKindComponentType, Name: "service"}}
	if !Matches(nsSel, nsSvc) {
		t.Fatal("a ComponentType selector must match a namespaced ComponentType subject")
	}
	if Matches(nsSel, clusterSvc) {
		t.Fatal("a ComponentType selector must not match a ClusterComponentType subject")
	}
	unknownSel := []openchoreov1alpha1.HookSubjectSelector{{Kind: "Component", Name: "service"}}
	if Matches(unknownSel, nsSvc) || Matches(unknownSel, clusterSvc) {
		t.Fatal("a selector of an unknown kind must match nothing")
	}

	nsRef := []openchoreov1alpha1.HookSubjectRef{{Kind: openchoreov1alpha1.HookSubjectRefKindComponentType, Name: "deployment/service"}}
	if !EnabledFor(nsRef, nsSvc) {
		t.Fatal("a ComponentType enabledTo ref must enable a namespaced ComponentType subject")
	}
	if EnabledFor(nsRef, clusterSvc) {
		t.Fatal("a ComponentType enabledTo ref must not enable a ClusterComponentType subject")
	}
}

// Names are compared exactly, with the "{workloadType}/" prefix of the subject
// optional on the selector side only; a bare subject name never matches a
// different bare selector.
func TestComponentTypeNameMatches(t *testing.T) {
	cases := []struct {
		selector, ref string
		want          bool
	}{
		{"service", "service", true},
		{"service", "deployment/service", true},
		{"deployment/service", "deployment/service", true},
		{"worker", "service", false},
		{"worker", "deployment/service", false},
		{"deployment", "deployment/service", false},
	}
	for _, tc := range cases {
		if got := componentTypeNameMatches(tc.selector, tc.ref); got != tc.want {
			t.Errorf("componentTypeNameMatches(%q, %q) = %v, want %v", tc.selector, tc.ref, got, tc.want)
		}
	}
}

// A binding whose appliesTo misses is NotApplicable even when the hook's
// enabledTo would also exclude it: the binding-side reason is the one the user
// controls, so it wins and is the one reported.
func TestResolveNotApplicableWinsOverNotEnabled(t *testing.T) {
	hook := clusterHookObj("svc-only")
	hook.Spec.EnabledTo = []openchoreov1alpha1.HookSubjectRef{{Kind: openchoreov1alpha1.HookSubjectRefKindComponentType, Name: "service"}}
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(hook).Build()
	env := environmentWith("prod", &openchoreov1alpha1.HookSet{
		PreDeploy: []openchoreov1alpha1.HookBinding{binding("scan",
			openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "svc-only"},
			openchoreov1alpha1.HookSubjectSelector{Kind: openchoreov1alpha1.HookSubjectSelectorKindComponentType, Name: "service"})},
	})
	set, err := Resolve(context.Background(), c, env, Subject{
		ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindComponentType, ComponentTypeName: "cronjob/task"})
	if err != nil {
		t.Fatal(err)
	}
	if got := set.PreDeploy[0]; got.SkipReason != SkipReasonNotApplicable || got.Hook == nil {
		t.Fatalf("got %+v, want SkipReason %q with the hook still attached", got, SkipReasonNotApplicable)
	}
}

// A hook whose parameter schema is not valid JSON cannot be hashed; the key
// must fail naming the binding whose hook is broken instead of hashing a
// partial representation that would hide later schema changes.
func TestKeyRejectsInvalidHookSchema(t *testing.T) {
	h := resolvedScan()
	h.Hook.Parameters[0].Schema = &runtime.RawExtension{Raw: []byte(`{not json`)}
	_, _, err := Key("rel", 0, EffectiveHookSet{PreDeploy: []ResolvedHook{h}})
	if err == nil || !strings.HasPrefix(err.Error(), `hook of binding "scan": `) {
		t.Fatalf("got %v, want an error prefixed with hook of binding \"scan\"", err)
	}
}

// A NotFound binding has no hook spec but must still contribute to the key, and
// the key must change once the hook appears so the gate re-evaluates it.
func TestKeyNotFoundBindingChangesWhenHookAppears(t *testing.T) {
	missing := resolvedScan()
	missing.Hook, missing.NotFound = nil, true
	kMissing, _, err := Key("rel", 0, EffectiveHookSet{PreDeploy: []ResolvedHook{missing}})
	if err != nil {
		t.Fatal(err)
	}
	kEmpty, _, err := Key("rel", 0, EffectiveHookSet{})
	if err != nil {
		t.Fatal(err)
	}
	kFound, _, err := Key("rel", 0, EffectiveHookSet{PreDeploy: []ResolvedHook{resolvedScan()}})
	if err != nil {
		t.Fatal(err)
	}
	if kMissing == kEmpty {
		t.Fatal("a NotFound binding must still contribute to the key")
	}
	if kMissing == kFound {
		t.Fatal("the key must change when a missing hook appears")
	}
}
