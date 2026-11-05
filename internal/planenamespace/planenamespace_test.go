// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package planenamespace

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return s
}

func namespace(name string, lbls map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls}}
}

func terminating(ns *corev1.Namespace) *corev1.Namespace {
	now := metav1.Now()
	ns.DeletionTimestamp = &now
	ns.Finalizers = []string{"test.openchoreo.dev/hold"}
	return ns
}

func names(t *testing.T, c client.Client) []string {
	t.Helper()
	var list corev1.NamespaceList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	var out []string
	for i := range list.Items {
		if list.Items[i].DeletionTimestamp.IsZero() {
			out = append(out, list.Items[i].Name)
		}
	}
	slices.Sort(out)
	return out
}

func getNamespace(t *testing.T, c client.Client, name string) *corev1.Namespace {
	t.Helper()
	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: name}, ns); err != nil {
		t.Fatalf("get namespace %s: %v", name, err)
	}
	return ns
}

func TestKindNames(t *testing.T) {
	if got := KV.Name("acme"); got != "openchoreo-kv-acme" {
		t.Errorf("KV.Name(acme) = %q", got)
	}
	if got := Workflow.Name("acme"); got != "workflows-acme" {
		t.Errorf("Workflow.Name(acme) = %q", got)
	}
}

func TestEnsure_CreatesLabeledNamespace(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()

	if err := Workflow.Ensure(context.Background(), c, "acme", "workflowrun-controller"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got := getNamespace(t, c, "workflows-acme").Labels
	want := map[string]string{
		"openchoreo.dev/workflow-namespace": "true",
		"openchoreo.dev/namespace":          "acme",
		"openchoreo.dev/created-by":         "workflowrun-controller",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("label %s = %q, want %q (labels %v)", k, got[k], v, got)
		}
	}
}

func TestEnsure_LabelsExistingNamespace(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(
		namespace("openchoreo-kv-acme", map[string]string{"team": "payments"}),
	).Build()

	if err := KV.Ensure(context.Background(), c, "acme", "openchoreo-api"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got := getNamespace(t, c, "openchoreo-kv-acme").Labels
	if got["openchoreo.dev/kv-namespace"] != "true" || got["openchoreo.dev/namespace"] != "acme" {
		t.Errorf("identifying labels not added: %v", got)
	}
	if got["team"] != "payments" {
		t.Errorf("existing label dropped: %v", got)
	}
	if _, ok := got["openchoreo.dev/created-by"]; ok {
		t.Errorf("created-by recorded on a namespace this caller did not create: %v", got)
	}
}

func TestEnsure_SkipsPatchWhenLabeled(t *testing.T) {
	patched := false
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(namespace("openchoreo-kv-acme", KV.Labels("acme"))).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				patched = true
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	if err := KV.Ensure(context.Background(), c, "acme", "openchoreo-api"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if patched {
		t.Error("namespace patched although it already had the identifying labels")
	}
}

func TestEnsure_Errors(t *testing.T) {
	notFound := func(_ context.Context, _ client.WithWatch, key client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
		return apierrors.NewNotFound(corev1.Resource("namespaces"), key.Name)
	}
	tests := []struct {
		name    string
		funcs   interceptor.Funcs
		wantErr bool
	}{
		{
			name: "get fails",
			funcs: interceptor.Funcs{Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("api server unavailable")
			}},
			wantErr: true,
		},
		{
			name: "create fails",
			funcs: interceptor.Funcs{Get: notFound, Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errors.New("quota exceeded")
			}},
			wantErr: true,
		},
		{
			name: "created concurrently",
			funcs: interceptor.Funcs{Get: notFound, Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(corev1.Resource("namespaces"), "workflows-acme")
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithInterceptorFuncs(tt.funcs).Build()
			err := Workflow.Ensure(context.Background(), c, "acme", "openchoreo-api")
			if (err != nil) != tt.wantErr {
				t.Errorf("Ensure error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteForNamespace(t *testing.T) {
	plane := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(
		namespace(KV.Name("acme"), KV.Labels("acme")),
		namespace(Workflow.Name("acme"), Workflow.Labels("acme")),
		namespace(KV.Name("other"), KV.Labels("other")),
		namespace(Workflow.Name("other"), Workflow.Labels("other")),
		// Environment namespaces carry the namespace label too.
		namespace("dp-acme-proj-dev-1234", map[string]string{"openchoreo.dev/namespace": "acme"}),
		// An unlabeled plane namespace is left for the cleanup script.
		namespace(Workflow.Name("legacy"), nil),
	).Build()

	if err := DeleteForNamespace(context.Background(), plane, "acme"); err != nil {
		t.Fatalf("DeleteForNamespace: %v", err)
	}

	want := []string{"dp-acme-proj-dev-1234", KV.Name("other"), Workflow.Name("legacy"), Workflow.Name("other")}
	slices.Sort(want)
	if got := names(t, plane); !slices.Equal(got, want) {
		t.Errorf("remaining namespaces = %v, want %v", got, want)
	}
}

func TestDeleteForNamespace_SkipsTerminating(t *testing.T) {
	deletes := 0
	plane := fake.NewClientBuilder().WithScheme(newScheme(t)).
		WithObjects(terminating(namespace(KV.Name("acme"), KV.Labels("acme")))).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				deletes++
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()

	if err := DeleteForNamespace(context.Background(), plane, "acme"); err != nil {
		t.Fatalf("DeleteForNamespace: %v", err)
	}
	if deletes != 0 {
		t.Errorf("deleted a plane namespace that was already terminating")
	}
}

func TestSweep(t *testing.T) {
	scheme := newScheme(t)
	plane := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		namespace(KV.Name("live"), KV.Labels("live")),
		namespace(Workflow.Name("live"), Workflow.Labels("live")),
		namespace(KV.Name("gone"), KV.Labels("gone")),
		namespace(Workflow.Name("gone"), Workflow.Labels("gone")),
		namespace(KV.Name("leaving"), KV.Labels("leaving")),
		namespace(KV.Name("new"), KV.Labels("new")),
		namespace(KV.Name("legacy"), nil),
	).Build()
	// The cache has not seen "new" yet; the live reader has.
	cached := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		namespace("live", nil),
		terminating(namespace("leaving", nil)),
	).Build()
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		namespace("live", nil),
		terminating(namespace("leaving", nil)),
		namespace("new", nil),
	).Build()

	deleted, err := Sweep(context.Background(), plane, cached, live)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	slices.Sort(deleted)
	wantDeleted := []string{KV.Name("gone"), KV.Name("leaving"), Workflow.Name("gone")}
	slices.Sort(wantDeleted)
	if !slices.Equal(deleted, wantDeleted) {
		t.Errorf("deleted = %v, want %v", deleted, wantDeleted)
	}
	wantRemaining := []string{KV.Name("legacy"), KV.Name("live"), KV.Name("new"), Workflow.Name("live")}
	slices.Sort(wantRemaining)
	if got := names(t, plane); !slices.Equal(got, wantRemaining) {
		t.Errorf("remaining namespaces = %v, want %v", got, wantRemaining)
	}
}

func TestSweep_ConfirmsCachedTerminatingNamespace(t *testing.T) {
	scheme := newScheme(t)
	plane := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace(KV.Name("acme"), KV.Labels("acme"))).Build()
	// The cache still holds the deleted namespace; the live reader has its recreation.
	cached := fake.NewClientBuilder().WithScheme(scheme).WithObjects(terminating(namespace("acme", nil))).Build()
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace("acme", nil)).Build()

	deleted, err := Sweep(context.Background(), plane, cached, live)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("deleted %v although the namespace exists again", deleted)
	}
}

func TestDeleteForNamespace_FollowsPages(t *testing.T) {
	scheme := newScheme(t)
	objs := make([]client.Object, 0, 4)
	objs = append(objs, namespace("unrelated", nil))
	for i := range 3 {
		objs = append(objs, namespace(fmt.Sprintf("%s-%d", KV.Name("acme"), i), KV.Labels("acme")))
	}
	// Serve paged LISTs one namespace per page from a snapshot taken on the first page.
	var snapshot []corev1.Namespace
	pages := 0
	plane := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			lo := (&client.ListOptions{}).ApplyOptions(opts)
			if lo.Limit == 0 {
				return c.List(ctx, list, opts...)
			}
			if lo.Continue == "" {
				all := &corev1.NamespaceList{}
				if err := c.List(ctx, all, client.MatchingLabelsSelector{Selector: lo.LabelSelector}); err != nil {
					return err
				}
				snapshot = all.Items
			}
			offset, _ := strconv.Atoi(lo.Continue)
			pages++
			page := list.(*corev1.NamespaceList)
			if offset < len(snapshot) {
				page.Items = snapshot[offset : offset+1]
			}
			if offset+1 < len(snapshot) {
				page.Continue = strconv.Itoa(offset + 1)
			}
			return nil
		},
	}).Build()

	if err := DeleteForNamespace(context.Background(), plane, "acme"); err != nil {
		t.Fatalf("DeleteForNamespace: %v", err)
	}
	if want := []string{"unrelated"}; !slices.Equal(names(t, plane), want) {
		t.Errorf("remaining namespaces = %v, want %v", names(t, plane), want)
	}
	// Three KV pages, plus one empty page for the workflow kind.
	if pages < 3 {
		t.Errorf("listed %d pages, want at least 3", pages)
	}
}

func TestSweep_KeepsNamespaceWhenLookupFails(t *testing.T) {
	scheme := newScheme(t)
	plane := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		namespace(KV.Name("unknown"), KV.Labels("unknown")),
		namespace(KV.Name("gone"), KV.Labels("gone")),
	).Build()
	cached := fake.NewClientBuilder().WithScheme(scheme).Build()
	live := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if key.Name == "unknown" {
				return apierrors.NewForbidden(corev1.Resource("namespaces"), key.Name, errors.New("denied"))
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()

	deleted, err := Sweep(context.Background(), plane, cached, live)
	if err == nil {
		t.Fatal("expected the lookup error to be returned")
	}
	if want := []string{KV.Name("gone")}; !slices.Equal(deleted, want) {
		t.Errorf("deleted = %v, want %v", deleted, want)
	}
	if got := names(t, plane); !slices.Contains(got, KV.Name("unknown")) {
		t.Errorf("plane namespace deleted although its namespace could not be looked up: %v", got)
	}
}

func TestSweep_ListError(t *testing.T) {
	plane := fake.NewClientBuilder().WithScheme(newScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return errors.New("plane unreachable")
		},
	}).Build()
	reader := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()

	if _, err := Sweep(context.Background(), plane, reader, reader); err == nil {
		t.Fatal("expected the list error to be returned")
	}
}
