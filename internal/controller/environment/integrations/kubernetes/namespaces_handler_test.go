// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return s
}

func envCtx(envName, orgNS string) *dataplane.EnvironmentContext {
	return &dataplane.EnvironmentContext{
		Environment: &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: envName, Namespace: orgNS},
		},
	}
}

func dpNS(name, env, orgNS string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				labels.LabelKeyEnvironmentName: env,
				labels.LabelKeyNamespaceName:   orgNS,
				labels.LabelKeyProjectName:     "my-project",
			},
		},
	}
}

func TestNamespacesHandler_GetCurrentState(t *testing.T) {
	s := testScheme(t)
	ctx := context.Background()
	ec := envCtx("development", "acme")

	t.Run("finds namespaces labeled with openchoreo.dev keys", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(s).WithObjects(
			dpNS("dp-acme-my-project-development-abc", "development", "acme"),
			dpNS("dp-acme-other-staging-xyz", "staging", "acme"),
			// Same env name under a different org namespace must not match.
			dpNS("dp-other-my-project-development-zzz", "development", "other-org"),
		).Build()
		h := NewNamespacesHandler(cli)

		got, err := h.GetCurrentState(ctx, ec)
		if err != nil {
			t.Fatalf("GetCurrentState: %v", err)
		}
		list, ok := got.(*corev1.NamespaceList)
		if !ok || list == nil {
			t.Fatalf("expected NamespaceList, got %T", got)
		}
		if len(list.Items) != 1 {
			t.Fatalf("expected 1 namespace, got %d", len(list.Items))
		}
		if list.Items[0].Name != "dp-acme-my-project-development-abc" {
			t.Fatalf("unexpected namespace %q", list.Items[0].Name)
		}
	})

	t.Run("ignores legacy unprefixed label keys", func(t *testing.T) {
		// Namespaces created by the current pipeline use openchoreo.dev/* labels.
		// Matching only the old keys would miss them and leave orphans on env delete.
		legacy := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dp-legacy",
				Labels: map[string]string{
					"environment-name": "development",
					"namespace-name":   "acme",
				},
			},
		}
		cli := fake.NewClientBuilder().WithScheme(s).WithObjects(legacy).Build()
		h := NewNamespacesHandler(cli)

		got, err := h.GetCurrentState(ctx, ec)
		if err != nil {
			t.Fatalf("GetCurrentState: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil for legacy labels, got %#v", got)
		}
	})

	t.Run("returns nil when no matching namespaces", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(s).Build()
		h := NewNamespacesHandler(cli)
		got, err := h.GetCurrentState(ctx, ec)
		if err != nil {
			t.Fatalf("GetCurrentState: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil, got %#v", got)
		}
	})
}

func TestNamespacesHandler_Delete(t *testing.T) {
	s := testScheme(t)
	ctx := context.Background()
	ec := envCtx("development", "acme")

	match := dpNS("dp-acme-my-project-development-abc", "development", "acme")
	otherEnv := dpNS("dp-acme-my-project-staging-def", "staging", "acme")
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(match, otherEnv).Build()
	h := NewNamespacesHandler(cli)

	if err := h.Delete(ctx, ec); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Matching namespace deleted
	err := cli.Get(ctx, client.ObjectKey{Name: match.Name}, &corev1.Namespace{})
	if err == nil {
		t.Fatal("expected matching namespace to be deleted")
	}

	// Other environment's namespace kept
	if err := cli.Get(ctx, client.ObjectKey{Name: otherEnv.Name}, &corev1.Namespace{}); err != nil {
		t.Fatalf("other env namespace should remain: %v", err)
	}
}
