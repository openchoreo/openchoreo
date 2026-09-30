// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

func strp(s string) *string { return &s }

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func hookObj(name string) *openchoreov1alpha1.Hook {
	return &openchoreov1alpha1.Hook{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: name},
		Spec: openchoreov1alpha1.HookSpec{
			Type:        openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: name + "-wf"},
			Parameters:  []openchoreov1alpha1.HookParameter{{Name: "image", From: "${deployment.workload.image}"}},
		},
	}
}

func clusterHookObj(name string) *openchoreov1alpha1.ClusterHook {
	return &openchoreov1alpha1.ClusterHook{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.HookSpec{
			Type:        openchoreov1alpha1.HookTypeWorkflow,
			WorkflowRef: &openchoreov1alpha1.WorkflowRef{Kind: openchoreov1alpha1.WorkflowRefKindClusterWorkflow, Name: name + "-wf"},
		},
	}
}

func binding(name string, ref openchoreov1alpha1.HookRef, applies ...openchoreov1alpha1.HookSubjectSelector) openchoreov1alpha1.HookBinding {
	return openchoreov1alpha1.HookBinding{Name: name, HookRef: ref, AppliesTo: applies}
}

// prodEnvironment binds three pre-deploy hooks and one post-deploy hook. The
// bindings are the ones the pipeline model used to reach "prod" from two paths,
// so the golden key in TestKeyStability also proves that moving bindings from a
// pipeline to the environment does not change the gate key.
func prodEnvironment() *openchoreov1alpha1.Environment {
	scan := binding("scan", openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "trivy"})
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "prod"},
		Spec: openchoreov1alpha1.EnvironmentSpec{Hooks: &openchoreov1alpha1.HookSet{
			PreDeploy: []openchoreov1alpha1.HookBinding{
				scan,
				binding("approval", openchoreov1alpha1.HookRef{Name: "approve"}),
				binding("hotfix-notice", openchoreov1alpha1.HookRef{Name: "notify"}),
			},
			PostDeploy: []openchoreov1alpha1.HookBinding{binding("smoke", openchoreov1alpha1.HookRef{Name: "smoke"})},
		}},
	}
}

func environmentWith(name string, hooks *openchoreov1alpha1.HookSet) *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: name},
		Spec:       openchoreov1alpha1.EnvironmentSpec{Hooks: hooks},
	}
}

// The effective set is the environment's bindings sorted by name. A missing
// hook is reported on the binding rather than aborting resolution, so the gate
// can block on exactly that binding.
func TestResolveEnvironmentBindings(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(hookObj("approve"), hookObj("smoke"), clusterHookObj("trivy")).Build()

	set, err := Resolve(context.Background(), c, prodEnvironment(), Subject{})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(set.PreDeploy))
	for _, h := range set.PreDeploy {
		names = append(names, h.Binding.Name)
	}
	if got := strings.Join(names, ","); got != "approval,hotfix-notice,scan" {
		t.Fatalf("pre-deploy = %q, want approval,hotfix-notice,scan", got)
	}
	if len(set.PostDeploy) != 1 || set.PostDeploy[0].Binding.Name != "smoke" {
		t.Fatalf("post-deploy = %+v", set.PostDeploy)
	}
	for _, h := range set.PreDeploy {
		switch h.Binding.Name {
		case "hotfix-notice":
			if !h.NotFound || h.Hook != nil {
				t.Fatalf("hotfix-notice should be NotFound, got %+v", h)
			}
		default:
			if h.NotFound || h.Hook == nil {
				t.Fatalf("%s should resolve, got %+v", h.Binding.Name, h)
			}
		}
	}
}

// An environment without hooks, or no environment at all, has no gate: every
// existing environment must keep deploying exactly as before.
func TestResolveNoHooks(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	for name, env := range map[string]*openchoreov1alpha1.Environment{
		"nil environment": nil,
		"no hooks":        environmentWith("staging", nil),
		"empty hook set":  environmentWith("staging", &openchoreov1alpha1.HookSet{}),
	} {
		t.Run(name, func(t *testing.T) {
			set, err := Resolve(context.Background(), c, env, Subject{})
			if err != nil {
				t.Fatal(err)
			}
			if !set.Empty() {
				t.Fatalf("expected no bindings, got %+v", set)
			}
		})
	}
}

// appliesTo is the only scoping mechanism in this cut (no component opt-out), so a
// selector must match both the kind and the name of the release's frozen component
// type, accepting the bare name or the "{workloadType}/{name}" reference form.
func TestMatchesAppliesTo(t *testing.T) {
	subject := Subject{
		ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
		ComponentTypeName: "deployment/service",
	}
	sel := func(k openchoreov1alpha1.HookSubjectSelectorKind, n string) openchoreov1alpha1.HookSubjectSelector {
		return openchoreov1alpha1.HookSubjectSelector{Kind: k, Name: n}
	}
	cases := []struct {
		name string
		sels []openchoreov1alpha1.HookSubjectSelector
		want bool
	}{
		{"empty applies to all", nil, true},
		{"cluster component type by bare name", []openchoreov1alpha1.HookSubjectSelector{sel(openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, "service")}, true},
		{"cluster component type by ref form", []openchoreov1alpha1.HookSubjectSelector{sel(openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, "deployment/service")}, true},
		{"wrong kind same name", []openchoreov1alpha1.HookSubjectSelector{sel(openchoreov1alpha1.HookSubjectSelectorKindComponentType, "service")}, false},
		{"right kind wrong name", []openchoreov1alpha1.HookSubjectSelector{sel(openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, "job")}, false},
		{"any selector matching is enough", []openchoreov1alpha1.HookSubjectSelector{
			sel(openchoreov1alpha1.HookSubjectSelectorKindComponentType, "job"),
			sel(openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType, "service")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Matches(tc.sels, subject); got != tc.want {
				t.Fatalf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}

// A binding whose appliesTo misses is Skipped but still part of the set, so the
// key is the same for every component in the environment.
func TestResolveMarksSkipped(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(clusterHookObj("trivy")).Build()
	env := environmentWith("prod", &openchoreov1alpha1.HookSet{
		PreDeploy: []openchoreov1alpha1.HookBinding{binding("scan",
			openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "trivy"},
			openchoreov1alpha1.HookSubjectSelector{Kind: openchoreov1alpha1.HookSubjectSelectorKindComponentType, Name: "service"})},
	})
	set, err := Resolve(context.Background(), c, env, Subject{
		ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindComponentType, ComponentTypeName: "cronjob/task"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.PreDeploy) != 1 || set.PreDeploy[0].Applicable() || set.PreDeploy[0].SkipReason != SkipReasonNotApplicable {
		t.Fatalf("expected one Skipped binding, got %+v", set.PreDeploy)
	}
}

// The gate key identifies one deployment attempt. It must be byte-stable across
// processes (golden), change when the release or any hook input changes, and
// only fold in the config digest when a binding triggers on ConfigChange —
// otherwise every env-config edit would re-run hooks nobody asked to re-run.
func TestKeyStability(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(hookObj("approve"), hookObj("smoke"), clusterHookObj("trivy")).Build()
	set, err := Resolve(context.Background(), c, prodEnvironment(), Subject{})
	if err != nil {
		t.Fatal(err)
	}

	key1, hash1, err := Key("web-bff-42", 0, set)
	if err != nil {
		t.Fatal(err)
	}
	const goldenKey = "865f1323d0770138798ea1584477a8e6da669d2e79ea539e52fe96ea703d0ac6"
	if key1 != goldenKey {
		t.Fatalf("key drifted: got %s, want %s (bump the golden only for an intentional key-format change)", key1, goldenKey)
	}

	key1b, hash1b, _ := Key("web-bff-42", 0, set)
	if key1b != key1 || hash1b != hash1 {
		t.Fatal("the key must be stable for identical inputs")
	}
	key3, _, _ := Key("web-bff-43", 0, set)
	if key3 == key1 {
		t.Fatal("a different release must produce a different key")
	}

	// A release that comes back after another was pinned is a new attempt: it
	// must get its own key, or it would reuse the earlier attempt's runs.
	keySeq, hashSeq, _ := Key("web-bff-42", 1, set)
	if keySeq == key1 {
		t.Fatal("a different attempt sequence must produce a different key")
	}
	if hashSeq != hash1 {
		t.Fatal("the attempt sequence must not change the hook set hash")
	}

	set.PreDeploy[0].Binding.Retries = 3
	keyA, hashA, _ := Key("web-bff-42", 0, set)
	if hashA == hash1 {
		t.Fatal("changing a binding must change the hook set hash")
	}
	if keyA == key1 {
		t.Fatal("a changed hook set must change the key")
	}
}

// Parameter precedence is the contract between platform engineers (hook authors)
// and pipeline authors (binding authors): a fixed value can never be overridden, a
// computed value only when the hook allows it, the binding fills open parameters,
// defaults fill the rest, and a required parameter left unset is an error rather
// than an empty string handed to the workflow.
func TestResolveParametersPrecedence(t *testing.T) {
	engine := template.NewEngine()
	inputs := map[string]any{"deployment": map[string]any{"workload": map[string]any{"image": "ghcr.io/x/app:1.2"}, "environment": map[string]any{"name": "prod"}}}
	hook := &openchoreov1alpha1.HookSpec{Parameters: []openchoreov1alpha1.HookParameter{
		{Name: "fixed", Value: strp("CRITICAL")},
		{Name: "image", From: "${deployment.workload.image}"},
		{Name: "env", From: "${deployment.environment.name}", Overridable: true},
		{Name: "threshold", Default: strp("5")},
		{Name: "ticket", Required: true},
		{Name: "optional"},
	}}
	bind := func(raw string) openchoreov1alpha1.HookBinding {
		b := openchoreov1alpha1.HookBinding{Name: "b"}
		if raw != "" {
			b.Parameters = &runtime.RawExtension{Raw: []byte(raw)}
		}
		return b
	}

	t.Run("full precedence", func(t *testing.T) {
		got, err := ResolveParameters(context.Background(), engine, hook, bind(`{"env":"canary","threshold":7,"ticket":"OPS-1"}`), inputs)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"fixed": "CRITICAL", "image": "ghcr.io/x/app:1.2", "env": "canary", "threshold": "7", "ticket": "OPS-1"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
			}
		}
	})
	t.Run("defaults and computed values when the binding is silent", func(t *testing.T) {
		got, err := ResolveParameters(context.Background(), engine, hook, bind(`{"ticket":"OPS-1"}`), inputs)
		if err != nil {
			t.Fatal(err)
		}
		if got["env"] != "prod" || got["threshold"] != "5" {
			t.Fatalf("got %v", got)
		}
		if _, ok := got["optional"]; ok {
			t.Fatal("an unset optional parameter must be omitted, not sent as empty")
		}
	})
	for name, raw := range map[string]string{
		"fixed cannot be overridden":   `{"fixed":"LOW","ticket":"x"}`,
		"non-overridable from":         `{"image":"evil","ticket":"x"}`,
		"required missing":             `{}`,
		"undeclared key":               `{"ticket":"x","typo":"1"}`,
		"parameters must be an object": `[1,2]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveParameters(context.Background(), engine, hook, bind(raw), inputs); err == nil {
				t.Fatalf("expected an error for %s", raw)
			}
		})
	}
}

// The deployment context is the hook author's only view of the release; the
// documented paths must resolve through CEL.
func TestBuildContextPaths(t *testing.T) {
	in := ContextInput{
		Release: &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "web-bff-42"},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner:            openchoreov1alpha1.ComponentReleaseOwner{ProjectName: "shop", ComponentName: "web-bff"},
				ComponentType:    openchoreov1alpha1.ComponentReleaseComponentType{Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType, Name: "deployment/service"},
				Workload:         openchoreov1alpha1.WorkloadTemplateSpec{Container: openchoreov1alpha1.Container{Image: "ghcr.io/x/app:1.2"}},
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{Parameters: &runtime.RawExtension{Raw: []byte(`{"replicas":3}`)}},
			},
		},
		Component:   &openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: "web-bff", Labels: map[string]string{"tier": "edge"}}},
		Project:     &openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "shop"}, Spec: openchoreov1alpha1.ProjectSpec{Type: openchoreov1alpha1.ProjectTypeRef{Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType, Name: "web"}}},
		Environment: &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "prod"}, Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: true}},
		Trigger:     openchoreov1alpha1.DeploymentTriggerReleaseChange,
		Endpoints: []openchoreov1alpha1.EndpointURLStatus{{Name: "api", InvokeURL: "https://api.example.com/shop",
			ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: "web-bff.svc", Port: 8080}}},
	}
	engine := template.NewEngine()
	inputs := BuildContext(in)
	for expr, want := range map[string]any{
		"${deployment.release}":                        "web-bff-42",
		"${deployment.component.name}":                 "web-bff",
		"${deployment.component.labels.tier}":          "edge",
		"${deployment.component.parameters.replicas}":  float64(3),
		"${deployment.componentType.name}":             "deployment/service",
		"${deployment.projectType.kind}":               "ClusterProjectType",
		"${deployment.project.name}":                   "shop",
		"${deployment.environment.isProduction}":       true,
		"${deployment.workload.containers.main.image}": "ghcr.io/x/app:1.2",
		"${deployment.trigger}":                        "ReleaseChange",
		"${deployment.endpoints.api.invokeURL}":        "https://api.example.com/shop",
		"${deployment.endpoints.api.serviceURL}":       "http://web-bff.svc:8080",
	} {
		got, err := engine.Render(context.Background(), expr, inputs)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if got != want {
			t.Fatalf("%s = %v (%T), want %v (%T)", expr, got, got, want, want)
		}
	}
}

// Editing the bound hook set must not un-deploy or re-gate the release that is
// already running: the new set applies from the next release change. A release
// change always re-opens the gate.
func TestCarryForward(t *testing.T) {
	passed := &openchoreov1alpha1.DeploymentGateStatus{Key: "k1", PassedKey: "k1", HookSetHash: "h1", LastPassedRelease: "r1"}
	if !CarryForward(passed, "r1", "h2") {
		t.Fatal("a passed gate must carry forward when only the hook set changed")
	}
	if CarryForward(passed, "r2", "h2") {
		t.Fatal("a release change is never carried forward")
	}
	if CarryForward(passed, "r1", "h1") {
		t.Fatal("an unchanged hook set means the key moved for another reason; no carry")
	}
	blocked := &openchoreov1alpha1.DeploymentGateStatus{Key: "k1", PassedKey: "k0", HookSetHash: "h1", LastPassedRelease: "r0"}
	if CarryForward(blocked, "r1", "h2") {
		t.Fatal("a gate that had not passed cannot be carried forward")
	}
}

// History is bounded and newest-first so the most recent passes survive the cap.
func TestHistoryFastPath(t *testing.T) {
	g := &openchoreov1alpha1.DeploymentGateStatus{}
	now := metav1.Now()
	for i := 0; i < MaxHistory+3; i++ {
		RecordPass(g, "k"+string(rune('a'+i)), "r"+string(rune('a'+i)), "h1", now)
	}
	if len(g.History) != MaxHistory {
		t.Fatalf("history len = %d, want %d", len(g.History), MaxHistory)
	}
	if g.History[0].Key != "km" || g.PassedKey != "km" || g.LastPassedRelease != "rm" {
		t.Fatalf("newest first expected, got %+v", g.History[0])
	}
	RecordPass(g, "km", "rm", "h1", now)
	if len(g.History) != MaxHistory || g.History[0].Key != "km" {
		t.Fatal("recording the same key twice must not duplicate it")
	}
}

// The trigger decides which bindings run and must not drift between reconciles
// of the same attempt: first deployment is BindingCreate, a new release is
// ReleaseChange, and a same-release re-evaluation with an unchanged hook set can
// only be a ConfigChange — even after the current key has been recorded as passed.
func TestCurrentTrigger(t *testing.T) {
	if got := CurrentTrigger(nil, "k1", "r1", "h1"); got != openchoreov1alpha1.DeploymentTriggerBindingCreate {
		t.Fatalf("nil gate: %s", got)
	}
	g := &openchoreov1alpha1.DeploymentGateStatus{History: []openchoreov1alpha1.GatePassRecord{{Key: "k1", Release: "r1", HookSetHash: "h1"}}}
	if got := CurrentTrigger(g, "k1", "r1", "h1"); got != openchoreov1alpha1.DeploymentTriggerBindingCreate {
		t.Fatalf("first key already recorded must still read as BindingCreate: %s", got)
	}
	if got := CurrentTrigger(g, "k2", "r2", "h1"); got != openchoreov1alpha1.DeploymentTriggerReleaseChange {
		t.Fatalf("new release: %s", got)
	}
	if got := CurrentTrigger(g, "k3", "r1", "h2"); got != openchoreov1alpha1.DeploymentTriggerReleaseChange {
		t.Fatalf("hook set change: %s", got)
	}
	metav1Now := metav1.Now()
	RecordPass(g, "k3", "r1", "h2", metav1Now)
	if got := CurrentTrigger(g, "k3", "r1", "h2"); got != openchoreov1alpha1.DeploymentTriggerReleaseChange {
		t.Fatalf("trigger must be stable after the key passes: %s", got)
	}
}

var _ client.Client = (client.Client)(nil)

// enabledTo is the platform engineer's own restriction on a hook; a binding can
// narrow it but never widen it, so a hook not enabled for the component's type
// must be Skipped even when the binding's appliesTo would have matched.
func TestEnabledForAndResolveNotEnabled(t *testing.T) {
	svc := Subject{ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType, ComponentTypeName: "deployment/service"}
	nsSvc := Subject{ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindComponentType, ComponentTypeName: "deployment/service"}
	task := Subject{ComponentTypeKind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType, ComponentTypeName: "cronjob/task"}
	enabled := []openchoreov1alpha1.HookSubjectRef{{Kind: openchoreov1alpha1.HookSubjectRefKindClusterComponentType, Name: "service"}}

	if !EnabledFor(nil, task) {
		t.Fatal("empty enabledTo must enable the hook for everything")
	}
	if !EnabledFor(enabled, svc) {
		t.Fatal("ClusterComponentType/service must match the bare name against the workloadType/name form")
	}
	if EnabledFor(enabled, nsSvc) {
		t.Fatal("a namespaced ComponentType of the same name must not match a ClusterComponentType ref")
	}
	if EnabledFor(enabled, task) {
		t.Fatal("a different type must not match")
	}

	hook := clusterHookObj("svc-only-scan")
	hook.Spec.EnabledTo = enabled
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(hook).Build()
	env := environmentWith("prod", &openchoreov1alpha1.HookSet{
		PreDeploy: []openchoreov1alpha1.HookBinding{binding("scan",
			openchoreov1alpha1.HookRef{Kind: openchoreov1alpha1.HookRefKindClusterHook, Name: "svc-only-scan"})},
	})
	set, err := Resolve(context.Background(), c, env, task)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.PreDeploy) != 1 || set.PreDeploy[0].Applicable() || set.PreDeploy[0].SkipReason != SkipReasonNotEnabled {
		t.Fatalf("expected the binding Skipped with NotEnabled for a task, got %+v", set.PreDeploy)
	}
	set, err = Resolve(context.Background(), c, env, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.PreDeploy) != 1 || !set.PreDeploy[0].Applicable() {
		t.Fatalf("expected the binding applicable for a service, got %+v", set.PreDeploy)
	}
}
