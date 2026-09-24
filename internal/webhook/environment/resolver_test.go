// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"errors"
	"slices"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// These tests use a fake client so the resolver's paths (default kind, caching,
// not-found vs API errors, missing workflows) run without envtest.

func fakeClient(t *testing.T, funcs *interceptor.Funcs, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreodevv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	b := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...)
	if funcs != nil {
		b = b.WithInterceptorFuncs(*funcs)
	}
	return b.Build()
}

func namespacedHook(name string, ref *openchoreodevv1alpha1.WorkflowRef, params ...openchoreodevv1alpha1.HookParameter) *openchoreodevv1alpha1.Hook {
	return &openchoreodevv1alpha1.Hook{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       openchoreodevv1alpha1.HookSpec{WorkflowRef: ref, Parameters: params},
	}
}

func causeFields(t *testing.T, err error) map[string]string {
	t.Helper()
	var status *apierrors.StatusError
	if !errors.As(err, &status) || !apierrors.IsInvalid(err) {
		t.Fatalf("err = %v, want an Invalid StatusError", err)
	}
	out := map[string]string{}
	for _, c := range status.ErrStatus.Details.Causes {
		out[c.Field] = c.Message
	}
	return out
}

// An empty hookRef kind means a namespaced Hook. Two bindings naming the same
// hook with "" and "Hook" are the same ref: it is fetched once, and a missing
// Workflow is reported once under the normalized "Hook/<name>" key, since the
// gate would fail closed on it at deployment time.
func TestResolverNormalizesKindAndWarnsOnMissingWorkflow(t *testing.T) {
	gets := 0
	c := fakeClient(t, &interceptor.Funcs{
		Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*openchoreodevv1alpha1.Hook); ok {
				gets++
			}
			return cl.Get(ctx, key, obj, opts...)
		},
	}, namespacedHook("smoke", &openchoreodevv1alpha1.WorkflowRef{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: "smoke-wf"}))

	env := environment("dev", &openchoreodevv1alpha1.HookSet{
		PostDeploy: []openchoreodevv1alpha1.HookBinding{
			{Name: "a", HookRef: openchoreodevv1alpha1.HookRef{Name: "smoke"}, Mode: openchoreodevv1alpha1.HookModeAsync},
			{Name: "b", HookRef: openchoreodevv1alpha1.HookRef{Kind: openchoreodevv1alpha1.HookRefKindHook, Name: "smoke"}, Mode: openchoreodevv1alpha1.HookModeAsync},
		},
	})
	warnings, err := (&Validator{Client: c}).ValidateCreate(context.Background(), env)
	if err != nil {
		t.Fatalf("a missing workflow must warn, not reject: %v", err)
	}
	want := admission.Warnings{
		`hook Hook/smoke has no workflow plane to run on: workflow 'smoke-wf' not found in namespace 'default': workflows.openchoreo.dev "smoke-wf" not found; a Sync pre-deploy binding will block with PlaneUnavailable`,
	}
	if !slices.Equal(warnings, want) {
		t.Fatalf("warnings = %q, want %q", warnings, want)
	}
	if gets != 1 {
		t.Fatalf("Hook fetched %d times, want 1 (cached per normalized ref)", gets)
	}
}

// Platform engineers may bind a namespaced hook before creating it; admission
// warns on the exact binding path instead of rejecting, so environments and
// hooks can be applied in any order.
func TestResolverMissingNamespacedHookWarns(t *testing.T) {
	c := fakeClient(t, nil)
	env := environment("dev", &openchoreodevv1alpha1.HookSet{
		PreDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "approve", HookRef: openchoreodevv1alpha1.HookRef{Name: "ghost"}}},
	})
	warnings, err := (&Validator{Client: c}).ValidateCreate(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	want := `spec.hooks.preDeploy[0].hookRef: Hook "ghost" not found; the binding will fail at deployment until it exists`
	if len(warnings) != 1 || warnings[0] != want {
		t.Fatalf("warnings = %q, want [%q]", warnings, want)
	}
}

// An API error other than not-found is not evidence the hook is missing. The
// binding is admitted with a "could not resolve" warning carrying the cause
// (rather than a misleading "not found"), for both hook kinds.
func TestResolverAPIErrorWarns(t *testing.T) {
	c := fakeClient(t, &interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return errors.New("apiserver unavailable")
		},
	})
	env := environment("dev", &openchoreodevv1alpha1.HookSet{
		PreDeploy: []openchoreodevv1alpha1.HookBinding{
			{Name: "scan", HookRef: clusterHookRef("trivy")},
			{Name: "approve", HookRef: openchoreodevv1alpha1.HookRef{Name: "approve"}},
		},
	})
	warnings, err := (&Validator{Client: c}).ValidateCreate(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	want := admission.Warnings{
		`spec.hooks.preDeploy[0].hookRef: could not resolve ClusterHook "trivy": apiserver unavailable`,
		`spec.hooks.preDeploy[1].hookRef: could not resolve Hook "approve": apiserver unavailable`,
	}
	if !slices.Equal(warnings, want) {
		t.Fatalf("warnings = %q, want %q", warnings, want)
	}
}

// A hook without a workflowRef has no plane to check; the resolver must not
// dereference it or invent a plane warning for it.
func TestResolverHookWithoutWorkflowRef(t *testing.T) {
	c := fakeClient(t, nil, namespacedHook("noop", nil))
	env := environment("dev", &openchoreodevv1alpha1.HookSet{
		PreDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "noop", HookRef: openchoreodevv1alpha1.HookRef{Name: "noop"}}},
	})
	warnings, err := (&Validator{Client: c}).ValidateCreate(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %q, want none", warnings)
	}
}

// Rules that need the resolved hook (parameters, enabledTo) must be reported on
// the binding that broke them, by index, so a user editing a long list can find
// it; the other bindings stay clean.
func TestBindingRulesReportedOnBindingPath(t *testing.T) {
	hook := namespacedHook("scan", nil,
		openchoreodevv1alpha1.HookParameter{Name: "ticket", Required: true},
		openchoreodevv1alpha1.HookParameter{Name: "image", Value: strPtr("fixed")},
	)
	hook.Spec.EnabledTo = []openchoreodevv1alpha1.HookSubjectRef{{Kind: openchoreodevv1alpha1.HookSubjectRefKindClusterComponentType, Name: "service"}}
	c := fakeClient(t, nil, hook)
	ref := openchoreodevv1alpha1.HookRef{Name: "scan"}

	env := environment("prod", &openchoreodevv1alpha1.HookSet{
		PreDeploy: []openchoreodevv1alpha1.HookBinding{
			{Name: "ok", HookRef: ref, Parameters: &runtime.RawExtension{Raw: []byte(`{"ticket":"OPS-1"}`)}},
			{Name: "no-ticket", HookRef: ref},
			{Name: "override", HookRef: ref, Parameters: &runtime.RawExtension{Raw: []byte(`{"ticket":"OPS-1","image":"x","extra":"y"}`)}},
			{Name: "wrong-type", HookRef: ref, Parameters: &runtime.RawExtension{Raw: []byte(`{"ticket":"OPS-1"}`)},
				AppliesTo: []openchoreodevv1alpha1.HookSubjectSelector{{Kind: openchoreodevv1alpha1.HookSubjectSelectorKindComponentType, Name: "service"}}},
		},
	})
	_, err := (&Validator{Client: c}).ValidateCreate(context.Background(), env)
	got := causeFields(t, err)
	want := map[string]string{
		"spec.hooks.preDeploy[1].parameters[ticket]": `Required value: parameter "ticket" is required by the hook`,
		"spec.hooks.preDeploy[2].parameters[image]":  `Forbidden: parameter "image" is fixed by the hook and cannot be overridden`,
		"spec.hooks.preDeploy[2].parameters[extra]":  `Invalid value: parameter "extra" is not declared by the hook`,
		"spec.hooks.preDeploy[3].appliesTo[0]":       `Invalid value: "ComponentType/service": the referenced hook is not enabled for this component type (see the hook's spec.enabledTo)`,
	}
	if len(got) != len(want) {
		t.Fatalf("causes = %v, want %v", got, want)
	}
	for f, msg := range want {
		if got[f] != msg {
			t.Errorf("cause %s = %q, want %q", f, got[f], msg)
		}
	}
}

// Update validates only the new object: an environment already stored with a
// bad binding can be fixed, and a good one cannot be broken. There are no
// update-only rules.
func TestValidateUpdateUsesNewObject(t *testing.T) {
	v := &Validator{}
	bad := environment("e", &openchoreodevv1alpha1.HookSet{
		PostDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "scan", HookRef: clusterHookRef("scan"), OnFailure: openchoreodevv1alpha1.HookFailurePolicyBlock}},
	})
	good := environment("e", nil)

	if _, err := v.ValidateUpdate(context.Background(), bad, good); err != nil {
		t.Fatalf("fixing a bad environment must be admitted: %v", err)
	}
	_, err := v.ValidateUpdate(context.Background(), good, bad)
	if _, ok := causeFields(t, err)["spec.hooks.postDeploy[0].onFailure"]; !ok {
		t.Fatalf("err = %v, want a cause on spec.hooks.postDeploy[0].onFailure", err)
	}
}
