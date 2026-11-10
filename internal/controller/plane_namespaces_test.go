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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
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
	require.NoError(t, scheme.AddToScheme(s))
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

// planeTestCRWithFinalizers creates a test CR with the given finalizers.
func planeTestCRWithFinalizers(namespace, name string, finalizers []string) *openchoreov1alpha1.DataPlane {
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: finalizers,
		},
	}
	return dp
}

// planeTestSecretRefWithFinalizer creates a SecretReference with a finalizer.
func planeTestSecretRefWithFinalizer(namespace, name, planeKind, planeName string, managed bool, finalizers []string) *openchoreov1alpha1.SecretReference {
	sr := planeTestSecretRef(namespace, name, planeKind, planeName, managed)
	sr.Finalizers = finalizers
	return sr
}

// TestFinalizeNamespacedPlane_MultipleFinalizersOnSameCR tests that when a CR
// has multiple finalizers, the namespace deletion waits for all finalizers to be removed.
func TestFinalizeNamespacedPlane_MultipleFinalizersOnSameCR(t *testing.T) {
	scheme := planeTestScheme(t)

	t.Run("blocks when CR has multiple finalizers and one remains", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			// Create a DataPlane with multiple finalizers
			&openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dp-multi",
					Namespace: testCPNamespace,
					Finalizers: []string{
						"openchoreo.dev/finalizer1",
						"openchoreo.dev/finalizer2",
					},
				},
			},
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		// Should not block on CR finalizers - the function only checks SecretReferences
		// The CR itself is managed by its own controller
		assert.Empty(t, msg)
	})

	t.Run("proceeds when CR is deleted but secret refs remain", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			planeTestSecretRef(testCPNamespace, "db", "DataPlane", "dp", true),
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		assert.Equal(t, "Deletion blocked: 1 secret(s) still target this plane", msg)
	})
}

// TestFinalizeNamespacedPlane_MultipleCRsWithFinalizers tests namespace deletion
// when multiple CRs with finalizers exist in the same namespace.
func TestFinalizeNamespacedPlane_MultipleCRsWithFinalizers(t *testing.T) {
	scheme := planeTestScheme(t)

	t.Run("blocks when multiple CRs with finalizers target the plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			planeTestSecretRef(testCPNamespace, "db1", "DataPlane", "dp", true),
			planeTestSecretRef(testCPNamespace, "db2", "DataPlane", "dp", true),
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		assert.Equal(t, "Deletion blocked: 2 secret(s) still target this plane", msg)
	})

	t.Run("proceeds when all secret refs for the plane are removed", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		assert.Empty(t, msg)
	})
}

// TestFinalizeNamespacedPlane_MultiplePlanesWithFinalizers tests namespace deletion
// when multiple planes with finalizers exist in the same namespace.
func TestFinalizeNamespacedPlane_MultiplePlanesWithFinalizers(t *testing.T) {
	scheme := planeTestScheme(t)

	t.Run("blocks when multiple planes have secrets targeting the same plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			// Two secrets targeting the same plane (dp)
			planeTestSecretRef(testCPNamespace, "dp-secret-1", "DataPlane", "dp", true),
			planeTestSecretRef(testCPNamespace, "dp-secret-2", "DataPlane", "dp", true),
		).Build()

		// Test plane "dp" - should see 2 secrets targeting it
		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		assert.Equal(t, "Deletion blocked: 2 secret(s) still target this plane", msg)
	})

	t.Run("proceeds when all planes' secret refs are removed", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		assert.Empty(t, msg)
	})
}

// TestFinalizeNamespacedPlane_GitSecretsAndAPISecrets tests that both git secrets
// and API-managed secrets are counted when blocking namespace deletion.
func TestFinalizeNamespacedPlane_GitSecretsAndAPISecrets(t *testing.T) {
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

	t.Run("counts both git secrets and API secrets", func(t *testing.T) {
		// The Secret API also creates git credentials, labeled with their category and targeting the plane.
		apiGitSecret := planeTestSecretRef(testCPNamespace, "api-git", "WorkflowPlane", "wp", true)
		apiGitSecret.Labels["openchoreo.dev/secret-type"] = "git-credentials"

		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			planeTestSecretRef(testCPNamespace, "regular-secret", "WorkflowPlane", "wp", true),
			gitSecret("WorkflowPlane", "wp"),
			gitSecret("ClusterWorkflowPlane", "wp"),
			apiGitSecret,
		).Build()

		wp := &openchoreov1alpha1.WorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "wp", Namespace: testCPNamespace}}
		planeCalled := false

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, wp, "WorkflowPlane",
			planeTestPlaneClient(&planeCalled, nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		// Should count: 1 regular secret + 1 git secret (WorkflowPlane) + 1 API git secret = 3 total
		// ClusterWorkflowPlane git secret doesn't match WorkflowPlane
		assert.Equal(t, "Deletion blocked: 3 secret(s) still target this plane", msg)
		assert.False(t, planeCalled)
	})
}

// TestFinalizeNamespacedPlane_SecretRefWithMultipleFinalizers tests that
// SecretReferences with their own finalizers are handled correctly.
func TestFinalizeNamespacedPlane_SecretRefWithMultipleFinalizers(t *testing.T) {
	scheme := planeTestScheme(t)

	t.Run("blocks when SecretReference has multiple finalizers", func(t *testing.T) {
		sr := planeTestSecretRefWithFinalizer(testCPNamespace, "multi-finalizer-secret", "DataPlane", "dp", true,
			[]string{"openchoreo.dev/finalizer1", "openchoreo.dev/finalizer2"})

		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace(testCPNamespace, false),
			sr,
		).Build()

		msg, err := FinalizeNamespacedPlane(context.Background(), cp, cp, planeTestDataPlane(), "DataPlane",
			planeTestPlaneClient(new(bool), nil, errors.New("must not reach the plane")))

		require.NoError(t, err)
		// Should count the secret ref regardless of its own finalizers
		assert.Equal(t, "Deletion blocked: 1 secret(s) still target this plane", msg)
	})
}

// TestFinalizeClusterPlane_MultipleFinalizers tests cluster plane finalization
// with multiple finalizers on cluster-scoped planes.
func TestFinalizeClusterPlane_MultipleFinalizers(t *testing.T) {
	scheme := planeTestScheme(t)

	t.Run("blocks when multiple secrets in different namespaces target the cluster plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestSecretRef("team-a", "db1", "ClusterDataPlane", "default", true),
			planeTestSecretRef("team-b", "db2", "ClusterDataPlane", "default", true),
			planeTestSecretRef("team-c", "db3", "ClusterDataPlane", "default", true),
		).Build()

		msg, err := FinalizeClusterPlane(context.Background(), cp, cp, "ClusterDataPlane", "default",
			func() (client.Client, error) { return nil, errors.New("must not reach the plane") })

		require.NoError(t, err)
		assert.Equal(t, "Deletion blocked: 3 secret(s) still target this plane", msg)
	})

	t.Run("sweeps all stale namespaces when no secrets target the plane", func(t *testing.T) {
		cp := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			planeTestCPNamespace("live1", false),
			planeTestCPNamespace("live2", false),
			planeTestCPNamespace("gone1", true), // terminating
			planeTestCPNamespace("gone2", true), // terminating
		).Build()

		plane := planeTestPlane(t, scheme,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: planenamespace.KV.Name("live1"), Labels: planenamespace.KV.Labels("live1")}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: planenamespace.KV.Name("live2"), Labels: planenamespace.KV.Labels("live2")}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: planenamespace.KV.Name("gone1"), Labels: planenamespace.KV.Labels("gone1")}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: planenamespace.KV.Name("gone2"), Labels: planenamespace.KV.Labels("gone2")}},
		)

		msg, err := FinalizeClusterPlane(context.Background(), cp, cp, "ClusterDataPlane", "default",
			func() (client.Client, error) { return plane, nil })

		require.NoError(t, err)
		assert.Empty(t, msg)
		assert.True(t, planeNamespaceExists(t, plane, planenamespace.KV.Name("live1")))
		assert.True(t, planeNamespaceExists(t, plane, planenamespace.KV.Name("live2")))
		assert.False(t, planeNamespaceExists(t, plane, planenamespace.KV.Name("gone1")))
		assert.False(t, planeNamespaceExists(t, plane, planenamespace.KV.Name("gone2")))
	})
}