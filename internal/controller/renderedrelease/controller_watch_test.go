// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ─────────────────────────────────────────────────────────────
// releasesForDataPlane / releasesForClusterDataPlane
// ─────────────────────────────────────────────────────────────

func TestReleasesForDataPlane(t *testing.T) {
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-a", Namespace: "ns1"},
	}
	envOnDP := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "env-1", Namespace: "ns1"},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "dp-a",
			},
		},
	}
	envOnOtherDP := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "env-2", Namespace: "ns1"},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "dp-b",
			},
		},
	}
	releaseOnEnv1 := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "rel-1", Namespace: "ns1"},
		Spec:       openchoreov1alpha1.RenderedReleaseSpec{EnvironmentName: "env-1"},
	}
	releaseOnEnv2 := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "rel-2", Namespace: "ns1"},
		Spec:       openchoreov1alpha1.RenderedReleaseSpec{EnvironmentName: "env-2"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(dataPlane, envOnDP, envOnOtherDP, releaseOnEnv1, releaseOnEnv2).
		Build()
	r := &Reconciler{Client: fakeClient}

	requests := r.releasesForDataPlane(context.Background(), dataPlane)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d: %+v", len(requests), requests)
	}
	if requests[0].Name != "rel-1" || requests[0].Namespace != "ns1" {
		t.Errorf("expected rel-1/ns1, got %+v", requests[0])
	}
}

func TestReleasesForClusterDataPlane(t *testing.T) {
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cdp-a"},
	}
	envOnCDP := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "env-1", Namespace: "ns1"},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
				Name: "cdp-a",
			},
		},
	}
	// Same name as the ClusterDataPlane but scoped to a namespaced DataPlane instead;
	// must not match since Kind differs.
	envOnDP := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: "env-2", Namespace: "ns1"},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "cdp-a",
			},
		},
	}
	releaseOnEnv1 := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "rel-1", Namespace: "ns1"},
		Spec:       openchoreov1alpha1.RenderedReleaseSpec{EnvironmentName: "env-1"},
	}
	releaseOnEnv2 := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "rel-2", Namespace: "ns1"},
		Spec:       openchoreov1alpha1.RenderedReleaseSpec{EnvironmentName: "env-2"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cdp, envOnCDP, envOnDP, releaseOnEnv1, releaseOnEnv2).
		Build()
	r := &Reconciler{Client: fakeClient}

	requests := r.releasesForClusterDataPlane(context.Background(), cdp)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d: %+v", len(requests), requests)
	}
	if requests[0].Name != "rel-1" || requests[0].Namespace != "ns1" {
		t.Errorf("expected rel-1/ns1, got %+v", requests[0])
	}
}

func TestReleasesForDataPlaneNoMatchingEnvironment(t *testing.T) {
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-unreferenced", Namespace: "ns1"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(dataPlane).Build()
	r := &Reconciler{Client: fakeClient}

	requests := r.releasesForDataPlane(context.Background(), dataPlane)
	if len(requests) != 0 {
		t.Errorf("expected no requests, got %+v", requests)
	}
}
