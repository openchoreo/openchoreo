// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/releaseline"
)

const testReleaseLine = "my-component-v2"

func buildMetadataContextForLine(line string) pipelinecontext.MetadataContext {
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	cr.Spec.Owner.ReleaseLine = line
	return newTestReconciler().buildMetadataContext(cr,
		&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{UID: "comp-uid"}},
		&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{UID: "proj-uid"}},
		&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp", UID: "dp-uid"}},
		&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{UID: "env-uid"}},
		testEnvStaging, lineUID(line))
}

// lineUID is the UID the tests give the ReleaseLine named line; empty without a line.
func lineUID(line string) string {
	if line == "" {
		return ""
	}
	return "uid-" + line
}

func TestBuildMetadataContext_DefaultLine(t *testing.T) {
	mctx := buildMetadataContextForLine("")

	assert.Empty(t, mctx.ReleaseLineName)
	assert.Equal(t, dpkubernetes.GenerateK8sName(testComponentName, testEnvStaging), mctx.Name)
	assert.NotContains(t, mctx.Labels, labels.LabelKeyReleaseLine)
	assert.NotContains(t, mctx.PodSelectors, labels.LabelKeyReleaseLine)
	assert.NotContains(t, mctx.PodSelectors, labels.LabelKeyReleaseLineUID)
	assert.Equal(t, "comp-uid", mctx.PodSelectors[labels.LabelKeyComponentUID])
}

func TestBuildMetadataContext_ReleaseLinesShareNamespaceWithDistinctNames(t *testing.T) {
	defaultLine := buildMetadataContextForLine("")
	v2 := buildMetadataContextForLine(testReleaseLine)

	assert.Equal(t, defaultLine.Namespace, v2.Namespace, "release lines render into the project namespace")

	assert.Equal(t, testReleaseLine, v2.ReleaseLineName)
	assert.Equal(t, releaseline.ObjectBaseName(testComponentName, testReleaseLine, testEnvStaging), v2.Name)
	assert.NotEqual(t, defaultLine.Name, v2.Name)

	assert.Equal(t, testReleaseLine, v2.Labels[labels.LabelKeyReleaseLine])
	assert.Equal(t, testReleaseLine, v2.PodSelectors[labels.LabelKeyReleaseLine])
	assert.Equal(t, lineUID(testReleaseLine), v2.Labels[labels.LabelKeyReleaseLineUID])
	assert.Equal(t, lineUID(testReleaseLine), v2.PodSelectors[labels.LabelKeyReleaseLineUID])
	assert.Equal(t, testComponentName, v2.Labels[labels.LabelKeyComponentName])
}

func TestBuildMetadataContext_SelectorsOfDefaultLineAndReleaseLineDoNotOverlap(t *testing.T) {
	defaultLine := buildMetadataContextForLine("")
	v2 := buildMetadataContextForLine(testReleaseLine)

	matches := func(selector, podLabels map[string]string) bool {
		for k, v := range selector {
			if podLabels[k] != v {
				return false
			}
		}
		return true
	}
	assert.False(t, matches(defaultLine.PodSelectors, v2.PodSelectors), "the default line's selector must not match a release line's pods")
	assert.False(t, matches(v2.PodSelectors, defaultLine.PodSelectors), "a release line's selector must not match the default line's pods")
	assert.NotContains(t, v2.PodSelectors, labels.LabelKeyComponentUID)
	assert.Equal(t, "comp-uid", v2.PodSelectors[labels.LabelKeyReleaseLineComponentUID], "a line's pods name their component under another key")
	assert.NotContains(t, defaultLine.PodSelectors, labels.LabelKeyReleaseLineComponentUID)
	assert.Equal(t, "comp-uid", v2.Labels[labels.LabelKeyComponentUID], "objects of a line keep the component UID label")
}

func TestMakeReleaseNames_ReleaseLine(t *testing.T) {
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	cr.Spec.Owner.ReleaseLine = testReleaseLine
	rb := makeValidReleaseBinding(testProjectName, testComponentName)
	rb.Spec.Environment = testEnvStaging

	base := releaseline.RenderedReleaseName(testComponentName, testReleaseLine, testEnvStaging)
	assert.Equal(t, base, makeDataPlaneReleaseName(cr, rb))
	assert.Equal(t, base+"-observability", makeObservabilityReleaseName(cr, rb))

	// The names label every object the releases apply, so they fit a label value however long the names are.
	long := makeValidComponentRelease(testProjectName, "customer-notification-service")
	long.Spec.Owner.ReleaseLine = "feature-checkout"
	assert.LessOrEqual(t, len(makeObservabilityReleaseName(long, rb)), 63)

	// Component "a-b" on line "c" must not share release names with component "a" on line "b-c",
	// or with component "a-b-c" without lines.
	names := func(component, line string) string {
		cr := makeValidComponentRelease(testProjectName, component)
		cr.Spec.Owner.ReleaseLine = line
		return makeDataPlaneReleaseName(cr, rb)
	}
	assert.NotEqual(t, names("a-b", "c"), names("a", "b-c"))
	assert.NotEqual(t, names("a-b", "c"), names("a-b-c", ""))
}

func TestValidateComponentRelease_ReleaseLineMismatch(t *testing.T) {
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	cr.Spec.Owner.ReleaseLine = testReleaseLine
	rb := makeValidReleaseBinding(testProjectName, testComponentName)

	err := newTestReconciler().validateComponentRelease(cr, rb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release line")

	rb.Spec.Owner.ReleaseLine = testReleaseLine
	assert.NoError(t, newTestReconciler().validateComponentRelease(cr, rb))
}

func TestValidateReleaseLine(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	line := &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: testReleaseLine, Namespace: "default"},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: testProjectName, ComponentName: testComponentName},
		},
	}
	r := &Reconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(line).Build(), Scheme: scheme}

	tests := []struct {
		name       string
		component  string
		line       string
		wantOK     bool
		wantReason controller.ConditionReason
	}{
		{name: "default line", component: testComponentName, line: "", wantOK: true},
		{name: "existing line of the component", component: testComponentName, line: testReleaseLine, wantOK: true},
		{name: "unknown line", component: testComponentName, line: "absent", wantReason: ReasonReleaseLineNotFound},
		{name: "line of another component", component: "other", line: testReleaseLine, wantReason: ReasonInvalidReleaseConfiguration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := makeValidReleaseBinding(testProjectName, tt.component)
			rb.Namespace = "default"
			rb.Spec.Owner.ReleaseLine = tt.line
			line, ok, err := r.validateReleaseLine(context.Background(), rb)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			if ok && tt.line != "" {
				require.NotNil(t, line)
				assert.Equal(t, tt.line, line.Name)
			} else {
				assert.Nil(t, line)
			}
			if !tt.wantOK {
				require.Len(t, rb.Status.Conditions, 1)
				assert.Equal(t, string(tt.wantReason), rb.Status.Conditions[0].Reason)
			}
		})
	}
}

func TestResolveConnection_SelectsTargetReleaseLine(t *testing.T) {
	newTarget := func(name, line, url string) *openchoreov1alpha1.ReleaseBinding {
		return &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName: testProjectName, ComponentName: testComponentName, ReleaseLine: line,
				},
				Environment: testEnvStaging,
			},
			Status: openchoreov1alpha1.ReleaseBindingStatus{
				Endpoints: []openchoreov1alpha1.EndpointURLStatus{{
					Name:       "http",
					ServiceURL: &openchoreov1alpha1.EndpointURL{Scheme: "http", Host: url, Port: 8080},
				}},
			},
		}
	}

	newComponent := func(name string) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName}},
		}
	}
	newLine := func(name, component string) *openchoreov1alpha1.ReleaseLine {
		return &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: testProjectName, ComponentName: component},
			},
		}
	}

	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(newTarget("main", "", "main.svc"), newTarget("v2", testReleaseLine, "v2.svc"),
			newComponent(testComponentName), newComponent("plain"), newComponent("lined"),
			newLine("canary", testComponentName), newLine("beta", "lined")).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{}, controller.IndexKeyReleaseBindingOwnerEnv, func(obj client.Object) []string {
			rb := obj.(*openchoreov1alpha1.ReleaseBinding)
			return []string{controller.MakeReleaseBindingOwnerEnvKey(rb.Spec.Owner.ProjectName, rb.Spec.Owner.ComponentName, rb.Spec.Environment)}
		}).
		Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	targetOf := func(component, line string) openchoreov1alpha1.ConnectionTarget {
		return openchoreov1alpha1.ConnectionTarget{
			Namespace: "default", Project: testProjectName, Component: component, ReleaseLine: line,
			Endpoint: "http", Visibility: openchoreov1alpha1.EndpointVisibilityProject, Environment: testEnvStaging,
		}
	}
	target := func(line string) openchoreov1alpha1.ConnectionTarget { return targetOf(testComponentName, line) }

	for line, wantHost := range map[string]string{"": "main.svc", testReleaseLine: "v2.svc"} {
		resolved, pending, err := r.resolveConnection(context.Background(), target(line))
		require.NoError(t, err)
		require.Nil(t, pending, "line %q", line)
		assert.Equal(t, wantHost, resolved.URL.Host, "line %q", line)
		assert.Equal(t, line, resolved.ReleaseLine)
	}

	for name, tc := range map[string]struct {
		target openchoreov1alpha1.ConnectionTarget
		want   string
	}{
		"line of a component without lines": {targetOf("plain", "next"), `release line "next" not found`},
		"unknown line":                      {target("ghost"), `release line "ghost" not found`},
		"line not deployed":                 {target("canary"), `release line "canary" of component my-project/my-component is not deployed in environment staging`},
		"default line not deployed":         {targetOf("lined", ""), "the default line of component my-project/lined is not deployed in environment staging"},
		"component not deployed":            {targetOf("plain", ""), "ReleaseBinding not found"},
		"unknown component":                 {targetOf("ghost", ""), "ReleaseBinding not found"},
	} {
		t.Run(name, func(t *testing.T) {
			resolved, pending, err := r.resolveConnection(context.Background(), tc.target)
			require.NoError(t, err)
			assert.Nil(t, resolved)
			require.NotNil(t, pending)
			assert.Contains(t, pending.Reason, tc.want)
		})
	}
}

func TestFindReleaseLineCollision(t *testing.T) {
	manifest := func(kind, apiVersion, name string) openchoreov1alpha1.RenderedManifest {
		raw, err := json.Marshal(map[string]any{
			"apiVersion": apiVersion, "kind": kind,
			"metadata": map[string]any{"name": name, "namespace": "dp-ns"},
		})
		require.NoError(t, err)
		return openchoreov1alpha1.RenderedManifest{ID: kind + "-" + name, Object: &runtime.RawExtension{Raw: raw}}
	}
	releaseOn := func(plane, name, component string, resources ...openchoreov1alpha1.RenderedManifest) *openchoreov1alpha1.RenderedRelease {
		return &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{
				labels.LabelKeyProjectName:     testProjectName,
				labels.LabelKeyComponentName:   component,
				labels.LabelKeyEnvironmentName: testEnvStaging,
			}},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: plane, Resources: resources},
		}
	}
	renderedRelease := func(name, component string, resources ...openchoreov1alpha1.RenderedManifest) *openchoreov1alpha1.RenderedRelease {
		return releaseOn(openchoreov1alpha1.TargetPlaneDataPlane, name, component, resources...)
	}

	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	r := &Reconciler{Scheme: scheme, Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		renderedRelease("comp-main-staging", testComponentName,
			manifest("Deployment", "apps/v1", "comp-main"), manifest("Service", "v1", "comp")),
		renderedRelease("other-main-staging", "other", manifest("Service", "v1", "shared")),
		releaseOn(openchoreov1alpha1.TargetPlaneObservabilityPlane, "comp-main-staging-observability", testComponentName,
			manifest("ObservabilityAlertRule", "openchoreo.dev/v1alpha1", "comp-high-error-rate")),
	).Build()}

	rb := makeValidReleaseBinding(testProjectName, testComponentName)
	rb.Namespace = "default"
	rb.Spec.Environment = testEnvStaging
	rb.Spec.Owner.ReleaseLine = testReleaseLine
	own := "comp-v2-staging"

	t.Run("distinct names", func(t *testing.T) {
		msg, err := r.findReleaseLineCollision(context.Background(), rb, own, openchoreov1alpha1.TargetPlaneDataPlane, []openchoreov1alpha1.RenderedManifest{
			manifest("Deployment", "apps/v1", "comp-v2"), manifest("Service", "v1", "comp-v2"),
			manifest("Service", "v1", "shared"),
		})
		require.NoError(t, err)
		assert.Empty(t, msg, "only releases of the same component are compared")
	})

	t.Run("same name on another line", func(t *testing.T) {
		msg, err := r.findReleaseLineCollision(context.Background(), rb, own, openchoreov1alpha1.TargetPlaneDataPlane, []openchoreov1alpha1.RenderedManifest{
			manifest("Deployment", "apps/v1", "comp-v2"), manifest("Service", "v1", "comp"),
		})
		require.NoError(t, err)
		assert.Contains(t, msg, "Service dp-ns/comp")
		assert.Contains(t, msg, `"comp-main-staging"`)
		assert.NotContains(t, msg, "Deployment")
	})

	t.Run("observability plane", func(t *testing.T) {
		alert := manifest("ObservabilityAlertRule", "openchoreo.dev/v1alpha1", "comp-high-error-rate")
		msg, err := r.findReleaseLineCollision(context.Background(), rb, own+"-observability",
			openchoreov1alpha1.TargetPlaneObservabilityPlane, []openchoreov1alpha1.RenderedManifest{alert})
		require.NoError(t, err)
		assert.Contains(t, msg, "ObservabilityAlertRule.openchoreo.dev dp-ns/comp-high-error-rate")
		assert.Contains(t, msg, `"comp-main-staging-observability"`)

		msg, err = r.findReleaseLineCollision(context.Background(), rb, own,
			openchoreov1alpha1.TargetPlaneDataPlane, []openchoreov1alpha1.RenderedManifest{alert})
		require.NoError(t, err)
		assert.Empty(t, msg, "releases on another plane are not compared")
	})

	t.Run("own release is ignored", func(t *testing.T) {
		msg, err := r.findReleaseLineCollision(context.Background(), rb, "comp-main-staging", openchoreov1alpha1.TargetPlaneDataPlane, []openchoreov1alpha1.RenderedManifest{
			manifest("Service", "v1", "comp"),
		})
		require.NoError(t, err)
		assert.Empty(t, msg)
	})
}

func TestFindDefaultLinePodLabel(t *testing.T) {
	manifest := func(obj map[string]any) openchoreov1alpha1.RenderedManifest {
		raw, err := json.Marshal(obj)
		require.NoError(t, err)
		return openchoreov1alpha1.RenderedManifest{ID: "r", Object: &runtime.RawExtension{Raw: raw}}
	}
	podTemplate := func(podLabels map[string]any) map[string]any {
		return map[string]any{"template": map[string]any{"metadata": map[string]any{"labels": podLabels}}}
	}
	lineLabels := map[string]any{labels.LabelKeyReleaseLine: testReleaseLine, labels.LabelKeyComponentName: "comp"}
	defaultLabels := map[string]any{labels.LabelKeyComponentUID: "c-1", labels.LabelKeyComponentName: "comp"}

	tests := []struct {
		name string
		obj  map[string]any
		want string
	}{
		{"deployment with line pod labels", map[string]any{"kind": "Deployment", "metadata": map[string]any{"name": "d"}, "spec": podTemplate(lineLabels)}, ""},
		{"service", map[string]any{"kind": "Service", "metadata": map[string]any{"name": "s", "labels": defaultLabels}}, ""},
		{"deployment with the component UID on its pods", map[string]any{"kind": "Deployment", "metadata": map[string]any{"name": "d"}, "spec": podTemplate(defaultLabels)}, `Deployment "d"`},
		{"cronjob with the component UID on its pods", map[string]any{"kind": "CronJob", "metadata": map[string]any{"name": "c"},
			"spec": map[string]any{"jobTemplate": map[string]any{"spec": podTemplate(defaultLabels)}}}, `CronJob "c"`},
		{"pod with the component UID", map[string]any{"kind": "Pod", "metadata": map[string]any{"name": "p", "labels": defaultLabels}}, `Pod "p"`},
		{"service selecting the line's pods", map[string]any{"kind": "Service", "metadata": map[string]any{"name": "s"},
			"spec": map[string]any{"selector": lineLabels}}, ""},
		{"service selecting on the component UID", map[string]any{"kind": "Service", "metadata": map[string]any{"name": "s"},
			"spec": map[string]any{"selector": defaultLabels}}, `Service "s"`},
		{"deployment selecting on the component UID", map[string]any{"kind": "Deployment", "metadata": map[string]any{"name": "d"},
			"spec": map[string]any{"selector": map[string]any{"matchLabels": defaultLabels}, "template": podTemplate(lineLabels)["template"]}}, `Deployment "d"`},
		{"network policy for the component UID", map[string]any{"kind": "NetworkPolicy", "metadata": map[string]any{"name": "n"},
			"spec": map[string]any{"podSelector": map[string]any{"matchExpressions": []any{
				map[string]any{"key": labels.LabelKeyComponentUID, "operator": "Exists"}}}}}, `NetworkPolicy "n"`},
		{"network policy excluding the component UID", map[string]any{"kind": "NetworkPolicy", "metadata": map[string]any{"name": "n"},
			"spec": map[string]any{"podSelector": map[string]any{"matchExpressions": []any{
				map[string]any{"key": labels.LabelKeyComponentUID, "operator": "DoesNotExist"}}}}}, ""},
		{"network policy peer selecting the component UID", map[string]any{"kind": "NetworkPolicy", "metadata": map[string]any{"name": "n"},
			"spec": map[string]any{"podSelector": map[string]any{"matchLabels": lineLabels},
				"ingress": []any{map[string]any{"from": []any{map[string]any{"podSelector": map[string]any{"matchLabels": defaultLabels}}}}}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := findDefaultLinePodLabel(testReleaseLine, []openchoreov1alpha1.RenderedManifest{manifest(tt.obj)})
			require.NoError(t, err)
			if tt.want == "" {
				assert.Empty(t, msg)
				return
			}
			assert.Contains(t, msg, tt.want)
			assert.Contains(t, msg, labels.LabelKeyComponentUID)
		})
	}
}

// Dependencies on two lines of one component's endpoint each get their own line's address.
func TestBuildConnectionItems_PerReleaseLine(t *testing.T) {
	resolvedOn := func(line, host string) openchoreov1alpha1.ResolvedConnection {
		return openchoreov1alpha1.ResolvedConnection{
			Namespace: testNamespace, Project: testProjectName, Component: "hello", ReleaseLine: line, Endpoint: "http",
			Visibility: openchoreov1alpha1.EndpointVisibilityProject,
			URL:        openchoreov1alpha1.EndpointURL{Scheme: "http", Host: host, Port: 9090},
		}
	}
	dependOn := func(line, env string) openchoreov1alpha1.WorkloadConnection {
		return openchoreov1alpha1.WorkloadConnection{
			Component: "hello", ReleaseLine: line, Name: "http",
			Visibility:  string(openchoreov1alpha1.EndpointVisibilityProject),
			EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{Address: env},
		}
	}
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace},
		Spec:       openchoreov1alpha1.ReleaseBindingSpec{Owner: openchoreov1alpha1.ReleaseBindingOwner{ProjectName: testProjectName}},
		Status: openchoreov1alpha1.ReleaseBindingStatus{ResolvedConnections: []openchoreov1alpha1.ResolvedConnection{
			resolvedOn("", "hello.dp.svc"), resolvedOn(testReleaseLine, "hello-v2.dp.svc"),
		}},
	}
	conns := []openchoreov1alpha1.WorkloadConnection{dependOn("", "HELLO_URL"), dependOn(testReleaseLine, "HELLO_V2_URL")}

	got := map[string]string{}
	for _, item := range buildConnectionItems(rb, conns) {
		for _, ev := range item.EnvVars {
			got[ev.Name] = ev.Value
		}
	}
	assert.Equal(t, map[string]string{"HELLO_URL": "http://hello.dp.svc:9090", "HELLO_V2_URL": "http://hello-v2.dp.svc:9090"}, got)
	assert.True(t, allConnectionsResolved(rb, conns))

	rb.Status.ResolvedConnections = rb.Status.ResolvedConnections[1:]
	assert.False(t, allConnectionsResolved(rb, conns), "the default line's dependency is unresolved while only the other line resolves")
}
