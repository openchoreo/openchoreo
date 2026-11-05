// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/planenamespace"
)

const testCPNamespace = "acme"

func planeTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, openchoreov1alpha1.AddToScheme(s))
	return s
}

func planeTestCPNamespace(name string, deleting bool) *corev1.Namespace {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if deleting {
		now := metav1.Now()
		ns.DeletionTimestamp = &now
		ns.Finalizers = []string{"test.openchoreo.dev/hold"}
	}
	return ns
}

func planeTestSecretRef(namespace, name, planeKind, planeName string, managed bool) *openchoreov1alpha1.SecretReference {
	sr := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKind, Name: planeName},
			Data: []openchoreov1alpha1.SecretDataSource{{
				SecretKey: "password",
				RemoteRef: openchoreov1alpha1.RemoteReference{Key: "k", Property: "password"},
			}},
		},
	}
	if managed {
		sr.Labels = map[string]string{"openchoreo.dev/managed-by": "openchoreo-api"}
	}
	return sr
}

func planeTestPlane(t *testing.T, scheme *runtime.Scheme, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func planeNamespaceExists(t *testing.T, plane client.Client, name string) bool {
	t.Helper()
	ns := &corev1.Namespace{}
	err := plane.Get(context.Background(), client.ObjectKey{Name: name}, ns)
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return ns.DeletionTimestamp.IsZero()
}

// planeTestPlaneClient records in called whether the plane was reached.
func planeTestPlaneClient(called *bool, c client.Client, err error) PlaneClientFunc {
	return func() (client.Client, error) {
		*called = true
		return c, err
	}
}

func planeTestDataPlane() *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: testCPNamespace}}
}

func TestFinalizeNamespacedPlane_NamespaceTerminating(t *testing.T) {
	scheme := planeTestScheme(t)
	cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		planeTestCPNamespace(testCPNamespace, true),
		planeTestSecretRef(testCPNamespace, "db", "DataPlane", "dp", true),
	).Build()
	plane := planeTestPlane(t, scheme,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: planenamespace.KV.Name(testCPNamespace), Labels: planenamespace.KV.Labels(testCPNamespace),
		}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: planenamespace.Workflow.Name(testCPNamespace), Labels: planenamespace.Workflow.Labels(testCPNamespace),
		}},
	)

	msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
		planeTestPlaneClient(new(bool), plane, nil))

	require.NoError(t, err)
	assert.Empty(t, msg, "a terminating namespace must not wait on its own secrets")
	assert.False(t, planeNamespaceExists(t, plane, planenamespace.KV.Name(testCPNamespace)))
	assert.False(t, planeNamespaceExists(t, plane, planenamespace.Workflow.Name(testCPNamespace)))
}

func TestFinalizeNamespacedPlane_ReadsNamespaceLive(t *testing.T) {
	scheme := planeTestScheme(t)
	// The cache has not seen the namespace start terminating yet.
	cached := fake.NewClientBuilder().WithScheme(scheme).WithObjects(planeTestCPNamespace(testCPNamespace, false)).Build()
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(planeTestCPNamespace(testCPNamespace, true)).Build()
	plane := planeTestPlane(t, scheme, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: planenamespace.KV.Name(testCPNamespace), Labels: planenamespace.KV.Labels(testCPNamespace),
	}})

	msg, err := FinalizeNamespacedPlane(context.Background(), cached, live, planeTestDataPlane(), "DataPlane",
		planeTestPlaneClient(new(bool), plane, nil))

	require.NoError(t, err)
	assert.Empty(t, msg)
	assert.False(t, planeNamespaceExists(t, plane, planenamespace.KV.Name(testCPNamespace)))
}

func TestFinalizeNamespacedPlane_WaitsForGitSecrets(t *testing.T) {
	scheme := planeTestScheme(t)
	gitSecret := func(planeKind, planeName string) *openchoreov1alpha1.SecretReference {
		sr := planeTestSecretRef(testCPNamespace, "git-"+planeKind+"-"+planeName, "", "", false)
		sr.Spec.TargetPlane = nil
		sr.Labels = map[string]string{
			"openchoreo.dev/secret-type":         "git-credentials",
			"openchoreo.dev/workflow-plane-kind": planeKind,
			"openchoreo.dev/workflow-plane-name": planeName,
		}
		return sr
	}
	// The Secret API also creates git credentials, labeled with their category and targeting the plane.
	apiGitSecret := planeTestSecretRef(testCPNamespace, "api-git", "WorkflowPlane", "wp", true)
	apiGitSecret.Labels["openchoreo.dev/secret-type"] = "git-credentials"
	cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		planeTestCPNamespace(testCPNamespace, false),
		gitSecret("WorkflowPlane", "wp"),
		gitSecret("ClusterWorkflowPlane", "wp"),
		apiGitSecret,
	).Build()
	wp := &openchoreov1alpha1.WorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "wp", Namespace: testCPNamespace}}
	planeCalled := false

	msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, wp, "WorkflowPlane",
		planeTestPlaneClient(&planeCalled, nil, errors.New("must not reach the plane")))

	require.NoError(t, err)
	assert.Equal(t, "Deletion blocked: 2 secret(s) still target this plane", msg)
	assert.False(t, planeCalled)
}

func TestFinalizeNamespacedPlane_WaitsForUnreachablePlane(t *testing.T) {
	scheme := planeTestScheme(t)
	cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(planeTestCPNamespace(testCPNamespace, true)).Build()

	msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
		planeTestPlaneClient(new(bool), nil, errors.New("agent disconnected")))

	require.NoError(t, err)
	assert.Contains(t, msg, "Waiting to delete the plane namespaces of acme")
	assert.Contains(t, msg, "agent disconnected")
}

func TestFinalizeNamespacedPlane_NamespaceStays(t *testing.T) {
	scheme := planeTestScheme(t)
	planeCalled := false
	planeClient := planeTestPlaneClient(&planeCalled, nil, errors.New("must not reach the plane"))

	t.Run("blocks while a secret targets the plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			planeTestSecretRef(testCPNamespace, "db", "DataPlane", "dp", true),
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane", planeClient)

		require.NoError(t, err)
		assert.Equal(t, "Deletion blocked: 1 secret(s) still target this plane", msg)
	})

	t.Run("proceeds when only other planes and unmanaged refs remain", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			planeTestSecretRef(testCPNamespace, "on-cluster-plane", "ClusterDataPlane", "dp", true),
			planeTestSecretRef(testCPNamespace, "other-plane", "DataPlane", "other", true),
			planeTestSecretRef(testCPNamespace, "hand-written", "DataPlane", "dp", false),
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane", planeClient)

		require.NoError(t, err)
		assert.Empty(t, msg)
	})

	assert.False(t, planeCalled, "the kv namespace must be left alone while its namespace stays")
}

func TestFinalizeClusterPlane(t *testing.T) {
	scheme := planeTestScheme(t)

	t.Run("blocks while a secret in any namespace targets the plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestSecretRef("team-a", "db", "ClusterDataPlane", "default", true),
			planeTestSecretRef("team-b", "db", "ClusterDataPlane", "default", true),
		).Build()

		msg, err := FinalizeClusterPlane(context.Background(), cp, cp, "ClusterDataPlane", "default",
			func() (client.Client, error) { return nil, errors.New("must not reach the plane") })

		require.NoError(t, err)
		assert.Equal(t, "Deletion blocked: 2 secret(s) still target this plane", msg)
	})

	t.Run("sweeps stale kv namespaces once no secret targets the plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(planeTestCPNamespace("live", false)).Build()
		plane := planeTestPlane(t, scheme,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: planenamespace.KV.Name("live"), Labels: planenamespace.KV.Labels("live")}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: planenamespace.KV.Name("gone"), Labels: planenamespace.KV.Labels("gone")}},
		)

		msg, err := FinalizeClusterPlane(context.Background(), cp, cp, "ClusterDataPlane", "default",
			func() (client.Client, error) { return plane, nil })

		require.NoError(t, err)
		assert.Empty(t, msg)
		assert.True(t, planeNamespaceExists(t, plane, planenamespace.KV.Name("live")))
		assert.False(t, planeNamespaceExists(t, plane, planenamespace.KV.Name("gone")))
	})

	t.Run("waits while the plane is unreachable", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).Build()

		msg, err := FinalizeClusterPlane(context.Background(), cp, cp, "ClusterDataPlane", "default",
			func() (client.Client, error) { return nil, errors.New("agent disconnected") })

		require.NoError(t, err)
		assert.Contains(t, msg, "agent disconnected")
	})
}

func TestControlPlaneNamespaceRemovedPredicate(t *testing.T) {
	p := ControlPlaneNamespaceRemovedPredicate()
	cpNS := func(deleting bool) *corev1.Namespace {
		ns := planeTestCPNamespace(testCPNamespace, deleting)
		ns.Labels = map[string]string{"openchoreo.dev/control-plane": "true"}
		return ns
	}
	plainNS := planeTestCPNamespace("kube-system", false)

	assert.False(t, p.Create(event.CreateEvent{Object: cpNS(false)}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: cpNS(false), ObjectNew: cpNS(true)}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: cpNS(true), ObjectNew: cpNS(true)}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: cpNS(false), ObjectNew: cpNS(false)}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: cpNS(true)}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: plainNS}))
}
