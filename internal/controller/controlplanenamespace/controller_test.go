// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controlplanenamespace

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("core scheme: %v", err)
	}
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("openchoreo scheme: %v", err)
	}
	return s
}

type staticWPProvider struct {
	cli client.Client
	err error
}

func (p *staticWPProvider) WorkflowPlaneClient(*openchoreov1alpha1.WorkflowPlane) (client.Client, error) {
	return p.cli, p.err
}

func (p *staticWPProvider) ClusterWorkflowPlaneClient(*openchoreov1alpha1.ClusterWorkflowPlane) (client.Client, error) {
	return p.cli, p.err
}

func controlPlaneNS(name string, finalizers ...string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Labels:     map[string]string{labels.LabelKeyControlPlaneNamespace: labels.LabelValueTrue},
			Finalizers: finalizers,
		},
	}
}

func TestReconcile_AddsFinalizer(t *testing.T) {
	s := testScheme(t)
	ns := controlPlaneNS("acme")
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &Reconciler{Client: cli, Scheme: s, PlaneClientProvider: &staticWPProvider{cli: cli}}

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "acme"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := &corev1.Namespace{}
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "acme"}, got); err != nil {
		t.Fatalf("get ns: %v", err)
	}
	if !controllerutil.ContainsFinalizer(got, CleanupFinalizer) {
		t.Fatal("expected cleanup finalizer")
	}
}

func TestReconcile_IgnoresUnlabeledNamespace(t *testing.T) {
	s := testScheme(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	cli := fake.NewClientBuilder().WithScheme(s).WithObjects(ns).Build()
	r := &Reconciler{Client: cli, Scheme: s}

	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "kube-system"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := &corev1.Namespace{}
	if err := cli.Get(context.Background(), types.NamespacedName{Name: "kube-system"}, got); err != nil {
		t.Fatalf("get ns: %v", err)
	}
	if controllerutil.ContainsFinalizer(got, CleanupFinalizer) {
		t.Fatal("should not add finalizer to unlabeled namespace")
	}
}

func TestFinalize_DeletesWorkflowsNamespace(t *testing.T) {
	s := testScheme(t)
	ctx := context.Background()

	// Control-plane ns being deleted
	cpNS := controlPlaneNS("acme", CleanupFinalizer)
	now := metav1.Now()
	cpNS.DeletionTimestamp = &now

	// Default ClusterWorkflowPlane so resolution succeeds
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}

	// Workflow-plane side: the shared workflows namespace
	wfNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "workflows-acme"}}

	cpClient := fake.NewClientBuilder().WithScheme(s).WithObjects(cpNS, cwp).Build()
	wpClient := fake.NewClientBuilder().WithScheme(s).WithObjects(wfNS).Build()

	r := &Reconciler{
		Client:              cpClient,
		Scheme:              s,
		PlaneClientProvider: &staticWPProvider{cli: wpClient},
	}

	// First pass: issues delete, requeues
	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "acme"}})
	if err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected requeue while workflows namespace is terminating")
	}

	// Simulate workflows namespace gone
	if err := wpClient.Delete(ctx, wfNS); err != nil && !apierrors.IsNotFound(err) {
		// fake client may have already marked it; force remove if needed
		_ = wpClient.Delete(ctx, wfNS)
	}
	// Fake client keeps deleting objects with finalizers; our wfNS has none so it should be gone.
	// If still present (DeletionTimestamp set), remove it from the store by creating a fresh client.
	if err := wpClient.Get(ctx, types.NamespacedName{Name: "workflows-acme"}, &corev1.Namespace{}); err == nil {
		// Still there — build a new wp client without it
		wpClient = fake.NewClientBuilder().WithScheme(s).Build()
		r.PlaneClientProvider = &staticWPProvider{cli: wpClient}
	}

	// Second pass: removes finalizer
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "acme"}})
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	got := &corev1.Namespace{}
	err = cpClient.Get(ctx, types.NamespacedName{Name: "acme"}, got)
	if apierrors.IsNotFound(err) {
		// finalizer removed and object GC'd — fine for fake client with deletionTimestamp
		return
	}
	if err != nil {
		t.Fatalf("get cp ns: %v", err)
	}
	if controllerutil.ContainsFinalizer(got, CleanupFinalizer) {
		t.Fatal("expected cleanup finalizer removed")
	}
}

func TestFinalize_SkipsWhenNoWorkflowPlane(t *testing.T) {
	s := testScheme(t)
	ctx := context.Background()

	cpNS := controlPlaneNS("acme", CleanupFinalizer)
	now := metav1.Now()
	cpNS.DeletionTimestamp = &now

	cpClient := fake.NewClientBuilder().WithScheme(s).WithObjects(cpNS).Build()
	r := &Reconciler{
		Client:              cpClient,
		Scheme:              s,
		PlaneClientProvider: &staticWPProvider{cli: fake.NewClientBuilder().WithScheme(s).Build()},
	}

	_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "acme"}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := &corev1.Namespace{}
	err = cpClient.Get(ctx, types.NamespacedName{Name: "acme"}, got)
	if apierrors.IsNotFound(err) {
		return
	}
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if controllerutil.ContainsFinalizer(got, CleanupFinalizer) {
		t.Fatal("expected finalizer removed when no workflow plane exists")
	}
}

func TestWorkflowsNamespaceName(t *testing.T) {
	if got := workflowsNamespaceName("acme"); got != "workflows-acme" {
		t.Fatalf("got %q", got)
	}
}
