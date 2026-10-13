// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"strings"
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
)

const testReleaseLine = "my-component-v2"

func buildMetadataContextForLine(component, line string) pipelinecontext.MetadataContext {
	cr := makeValidComponentRelease(testProjectName, component)
	cr.Spec.Owner.ReleaseLine = line
	return newTestReconciler().buildMetadataContext(cr,
		&openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{UID: "comp-uid"}},
		&openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{UID: "proj-uid"}},
		&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp", UID: "dp-uid"}},
		&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{UID: "env-uid"}},
		testEnvStaging)
}

func TestBuildMetadataContext_WithoutReleaseLine(t *testing.T) {
	mctx := buildMetadataContextForLine(testComponentName, "")

	assert.Empty(t, mctx.ReleaseLine)
	assert.Equal(t, testComponentName, mctx.ReleaseLineName)
	assert.Equal(t, dpkubernetes.GenerateK8sName(testComponentName, testEnvStaging), mctx.Name)
	assert.NotContains(t, mctx.Labels, labels.LabelKeyReleaseLine)
	assert.NotContains(t, mctx.PodSelectors, labels.LabelKeyReleaseLine)
}

func TestBuildMetadataContext_ReleaseLinesShareNamespaceWithDistinctNames(t *testing.T) {
	unlined := buildMetadataContextForLine(testComponentName, "")
	mainLine := buildMetadataContextForLine(testComponentName, "main")
	v2 := buildMetadataContextForLine(testComponentName, testReleaseLine)

	assert.Equal(t, unlined.Namespace, v2.Namespace, "release lines render into the project namespace")
	assert.Equal(t, unlined.Namespace, mainLine.Namespace)

	assert.Equal(t, testReleaseLine, v2.ReleaseLine)
	assert.Equal(t, testComponentName+"-"+testReleaseLine, v2.ReleaseLineName)
	assert.Equal(t, dpkubernetes.GenerateK8sName(testComponentName, testReleaseLine, testEnvStaging), v2.Name)
	assert.NotEqual(t, mainLine.Name, v2.Name)
	assert.NotEqual(t, mainLine.ReleaseLineName, v2.ReleaseLineName)

	assert.Equal(t, testReleaseLine, v2.Labels[labels.LabelKeyReleaseLine])
	assert.Equal(t, testReleaseLine, v2.PodSelectors[labels.LabelKeyReleaseLine])
	assert.Equal(t, "main", mainLine.PodSelectors[labels.LabelKeyReleaseLine])
	assert.Equal(t, testComponentName, v2.Labels[labels.LabelKeyComponentName])
}

func TestBuildMetadataContext_ReleaseLineNameFitsServiceName(t *testing.T) {
	component := strings.Repeat("c", 40)
	line := strings.Repeat("l", 40)

	a := buildMetadataContextForLine(component, line)
	b := buildMetadataContextForLine(component, line+"x")

	assert.LessOrEqual(t, len(a.ReleaseLineName), dpkubernetes.MaxServiceNameLength)
	assert.NotEqual(t, a.ReleaseLineName, b.ReleaseLineName)
}

func TestMakeReleaseNames_ReleaseLine(t *testing.T) {
	cr := makeValidComponentRelease(testProjectName, testComponentName)
	cr.Spec.Owner.ReleaseLine = testReleaseLine
	rb := makeValidReleaseBinding(testProjectName, testComponentName)
	rb.Spec.Environment = testEnvStaging

	assert.Equal(t, testComponentName+"-"+testReleaseLine+"-"+testEnvStaging, makeDataPlaneReleaseName(cr, rb))
	assert.Equal(t, testComponentName+"-"+testReleaseLine+"-"+testEnvStaging+"-observability", makeObservabilityReleaseName(cr, rb))
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
		enabled    bool
		line       string
		wantOK     bool
		wantReason controller.ConditionReason
	}{
		{name: "component without release lines", component: testComponentName, line: "", wantOK: true},
		{name: "line on a component without release lines", component: testComponentName, line: testReleaseLine, wantReason: ReasonInvalidReleaseConfiguration},
		{name: "missing line on a release-line component", component: testComponentName, enabled: true, line: "", wantReason: ReasonInvalidReleaseConfiguration},
		{name: "existing line of the component", component: testComponentName, enabled: true, line: testReleaseLine, wantOK: true},
		{name: "unknown line", component: testComponentName, enabled: true, line: "absent", wantReason: ReasonReleaseLineNotFound},
		{name: "line of another component", component: "other", enabled: true, line: testReleaseLine, wantReason: ReasonInvalidReleaseConfiguration},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := makeValidReleaseBinding(testProjectName, tt.component)
			rb.Namespace = "default"
			rb.Spec.Owner.ReleaseLine = tt.line
			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: tt.component, Namespace: "default"},
				Spec:       openchoreov1alpha1.ComponentSpec{ReleaseLinesEnabled: tt.enabled},
			}

			ok, err := r.validateReleaseLine(context.Background(), rb, comp)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
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

	newComponent := func(name string, lined bool) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:               openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ReleaseLinesEnabled: lined,
			},
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
		WithObjects(newTarget("main", "main", "main.svc"), newTarget("v2", testReleaseLine, "v2.svc"),
			newComponent(testComponentName, true), newComponent("plain", false), newLine("canary", testComponentName)).
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

	for line, wantHost := range map[string]string{"main": "main.svc", testReleaseLine: "v2.svc"} {
		resolved, pending, err := r.resolveConnection(context.Background(), target(line))
		require.NoError(t, err)
		require.Nil(t, pending, "line %q", line)
		assert.Equal(t, wantHost, resolved.URL.Host, "line %q", line)
		assert.Equal(t, line, resolved.ReleaseLine)
	}

	resolved, pending, err := r.resolveConnection(context.Background(), target(""))
	require.NoError(t, err)
	assert.Nil(t, resolved, "a release-line component has no implicit line to fall back to")
	require.NotNil(t, pending)
	assert.Contains(t, pending.Reason, "set releaseLine on the dependency")

	for name, tc := range map[string]struct {
		target openchoreov1alpha1.ConnectionTarget
		want   string
	}{
		"line on a component without lines": {targetOf("plain", "next"), "does not have release lines enabled"},
		"unknown line":                      {target("ghost"), `release line "ghost" not found`},
		"line not deployed":                 {target("canary"), `release line "canary" of component my-project/my-component is not deployed in environment staging`},
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
	renderedRelease := func(name, component string, resources ...openchoreov1alpha1.RenderedManifest) *openchoreov1alpha1.RenderedRelease {
		return &openchoreov1alpha1.RenderedRelease{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{
				labels.LabelKeyProjectName:     testProjectName,
				labels.LabelKeyComponentName:   component,
				labels.LabelKeyEnvironmentName: testEnvStaging,
			}},
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: openchoreov1alpha1.TargetPlaneDataPlane, Resources: resources},
		}
	}

	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	r := &Reconciler{Scheme: scheme, Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		renderedRelease("comp-main-staging", testComponentName,
			manifest("Deployment", "apps/v1", "comp-main"), manifest("Service", "v1", "comp")),
		renderedRelease("other-main-staging", "other", manifest("Service", "v1", "shared")),
	).Build()}

	rb := makeValidReleaseBinding(testProjectName, testComponentName)
	rb.Namespace = "default"
	rb.Spec.Environment = testEnvStaging
	rb.Spec.Owner.ReleaseLine = testReleaseLine
	own := "comp-v2-staging"

	t.Run("distinct names", func(t *testing.T) {
		msg, err := r.findReleaseLineCollision(context.Background(), rb, own, []openchoreov1alpha1.RenderedManifest{
			manifest("Deployment", "apps/v1", "comp-v2"), manifest("Service", "v1", "comp-v2"),
			manifest("Service", "v1", "shared"),
		})
		require.NoError(t, err)
		assert.Empty(t, msg, "only releases of the same component are compared")
	})

	t.Run("same name on another line", func(t *testing.T) {
		msg, err := r.findReleaseLineCollision(context.Background(), rb, own, []openchoreov1alpha1.RenderedManifest{
			manifest("Deployment", "apps/v1", "comp-v2"), manifest("Service", "v1", "comp"),
		})
		require.NoError(t, err)
		assert.Contains(t, msg, "Service dp-ns/comp")
		assert.Contains(t, msg, `"comp-main-staging"`)
		assert.NotContains(t, msg, "Deployment")
	})

	t.Run("own release is ignored", func(t *testing.T) {
		msg, err := r.findReleaseLineCollision(context.Background(), rb, "comp-main-staging", []openchoreov1alpha1.RenderedManifest{
			manifest("Service", "v1", "comp"),
		})
		require.NoError(t, err)
		assert.Empty(t, msg)
	})
}
