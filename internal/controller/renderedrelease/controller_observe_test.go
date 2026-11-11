// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// ─────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────

func observedRes(id, apiVersion, kind, namespace, name string) openchoreov1alpha1.ObservedResource {
	return openchoreov1alpha1.ObservedResource{ID: id, APIVersion: apiVersion, Kind: kind, Namespace: namespace, Name: name}
}

// readOnlyPlaneClient builds a fake plane client holding objs that fails the test
// on any write. getErr, when set, decides the result of a Get before the store
// is consulted; returning nil falls through to the store.
func readOnlyPlaneClient(t *testing.T, getErr func(key client.ObjectKey, obj client.Object) error,
	objs ...client.Object) client.Client {
	t.Helper()
	write := func(op string) error {
		t.Errorf("observed resources must never be written, got a %s", op)
		return fmt.Errorf("unexpected %s", op)
	}
	return fake.NewClientBuilder().WithObjects(objs...).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if getErr != nil {
				if err := getErr(key, obj); err != nil {
					return err
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return write("create")
		},
		Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
			return write("update")
		},
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return write("patch")
		},
		Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
			return write("delete")
		},
	}).Build()
}

func unstructuredObj(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

// ─────────────────────────────────────────────────────────────
// entries
// ─────────────────────────────────────────────────────────────

func TestResolveObservedResources(t *testing.T) {
	t.Run("keeps ids as written, in spec order, with apiVersion parsed", func(t *testing.T) {
		entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
			observedRes("api", "apps/v1", "Deployment", "prod", "api"),
			observedRes("api-svc", "v1", "Service", "prod", "api"),
			observedRes("ns", "v1", "Namespace", "", "prod"),
		})
		if len(entries) != 3 {
			t.Fatalf("got %d entries, want 3", len(entries))
		}
		for i, want := range []string{"api", "api-svc", "ns"} {
			if entries[i].id != want {
				t.Errorf("entries[%d].id = %q, want %q", i, entries[i].id, want)
			}
		}
		if got := entries[0].gvk; got != (schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}) {
			t.Errorf("gvk = %v", got)
		}
		if got := entries[1].gvk; got != (schema.GroupVersionKind{Version: "v1", Kind: "Service"}) {
			t.Errorf("core gvk = %v", got)
		}
		if entries[2].namespace != "" {
			t.Errorf("cluster-scoped entry got namespace %q", entries[2].namespace)
		}
	})

	t.Run("an unparseable apiVersion is kept for the read to report", func(t *testing.T) {
		entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
			observedRes("bad", "a/b/c", "Thing", "prod", "x")})
		if entries[0].apiVersionErr == nil {
			t.Error("expected an apiVersion error on the entry")
		}
	})
}

// reading
// ─────────────────────────────────────────────────────────────

func TestReadObservedResources(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}

	deploy := healthyDeployment(4, 4)
	deploy.SetNamespace("prod")
	deploy.SetName("api")
	deploy.SetLabels(nil)
	settings := unstructuredObj("v1", "ConfigMap", "prod", "settings")

	// Errors a real plane returns for the entries named "forbidden", "nomatch",
	// "unserved" and "broken"; everything else comes from the store.
	forbidden := apierrors.NewForbidden(schema.GroupResource{Group: "example.com", Resource: "widgets"}, "forbidden",
		errors.New(`User "system:serviceaccount:openchoreo-data-plane:cluster-agent" cannot get resource`))
	unserved := &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotFound, Code: 404,
		Message: "the server could not find the requested resource", Details: &metav1.StatusDetails{},
	}}
	getErr := func(key client.ObjectKey, _ client.Object) error {
		switch key.Name {
		case "forbidden":
			return forbidden
		case "nomatch":
			return &apimeta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "example.com", Kind: "Gadget"}}
		case "unserved":
			return unserved
		case "broken":
			return errors.New("tunnel closed")
		}
		return nil
	}
	planeClient := readOnlyPlaneClient(t, getErr, deploy, settings)

	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "Deployment", "prod", "api"),
		observedRes("settings", "v1", "ConfigMap", "prod", "settings"),
		observedRes("gone", "v1", "Service", "prod", "gone"),
		observedRes("forbidden", "example.com/v1", "Widget", "prod", "forbidden"),
		observedRes("nomatch", "example.com/v1", "Gadget", "prod", "nomatch"),
		observedRes("unserved", "example.com/v1", "Doohickey", "prod", "unserved"),
		observedRes("broken", "v1", "ConfigMap", "prod", "broken"),
	})
	result := r.readObservedResources(ctx, planeClient, &openchoreov1alpha1.RenderedRelease{}, entries)

	if len(result.statuses) != len(entries) {
		t.Fatalf("got %d statuses, want one per entry (%d)", len(result.statuses), len(entries))
	}
	for i, s := range result.statuses {
		if s.ID != entries[i].id {
			t.Errorf("statuses[%d].ID = %q, want %q (spec order)", i, s.ID, entries[i].id)
		}
	}

	t.Run("a Deployment is judged by the Deployment check", func(t *testing.T) {
		s := result.statuses[0]
		if s.HealthStatus != openchoreov1alpha1.HealthStatusHealthy {
			t.Errorf("health = %q, want Healthy", s.HealthStatus)
		}
		if s.Group != "apps" || s.Version != "v1" || s.Kind != "Deployment" || s.Namespace != "prod" || s.Name != "api" {
			t.Errorf("identity = %+v", s)
		}
		if s.Status == nil || !strings.Contains(string(s.Status.Raw), `"availableReplicas":1`) {
			t.Errorf("status was not copied: %v", s.Status)
		}
		if s.LastObservedTime == nil {
			t.Error("LastObservedTime not set for an object that was read")
		}
	})

	t.Run("a kind without a check is Healthy when it exists", func(t *testing.T) {
		if got := result.statuses[1].HealthStatus; got != openchoreov1alpha1.HealthStatusHealthy {
			t.Errorf("health = %q, want Healthy", got)
		}
	})

	// detail is checked exactly; "broken" carries the error text, checked below.
	wantFailures := []struct {
		id     string
		reason controller.ConditionReason
		detail string
	}{
		{"gone", ReasonObservedResourceNotFound, "not found"},
		{"forbidden", ReasonObservedReadForbidden, "forbidden by the plane agent's RBAC"},
		{"nomatch", ReasonObservedKindNotFound, "kind is not served by the plane"},
		// A 404 naming nothing also comes back for a request at the wrong scope.
		{"unserved", ReasonObservedKindNotFound,
			"kind is not served by the plane, or the namespace does not fit the kind's scope"},
		{"broken", ReasonObservedReadFailed, ""},
	}
	t.Run("unreadable objects are Unknown with a reason", func(t *testing.T) {
		if len(result.failures) != len(wantFailures) {
			t.Fatalf("got %d failures, want %d: %+v", len(result.failures), len(wantFailures), result.failures)
		}
		for i, want := range wantFailures {
			f := result.failures[i]
			if f.entry.id != want.id || f.reason != want.reason {
				t.Errorf("failures[%d] = %s/%s, want %s/%s", i, f.entry.id, f.reason, want.id, want.reason)
			}
			if want.detail != "" && f.detail != want.detail {
				t.Errorf("failures[%d] detail = %q, want %q", i, f.detail, want.detail)
			}
			s := result.statuses[i+2]
			if s.HealthStatus != openchoreov1alpha1.HealthStatusUnknown || s.Status != nil || s.LastObservedTime != nil {
				t.Errorf("status for %s = %+v, want Unknown with no status or time", want.id, s)
			}
		}
		if !strings.Contains(result.failures[4].detail, "tunnel closed") {
			t.Errorf("ReadFailed detail = %q, want the error", result.failures[4].detail)
		}
	})

	t.Run("only what was read is returned as live", func(t *testing.T) {
		if len(result.live) != 2 {
			t.Errorf("got %d live objects, want 2", len(result.live))
		}
	})
}

func TestReadObservedResourcesKeepsLastObservedTime(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deploy := healthyDeployment(2, 2)
	deploy.SetNamespace("prod")
	deploy.SetName("api")
	planeClient := readOnlyPlaneClient(t, nil, deploy)
	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "Deployment", "prod", "api")})

	first := r.readObservedResources(ctx, planeClient, &openchoreov1alpha1.RenderedRelease{}, entries)
	earlier := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
	old := &openchoreov1alpha1.RenderedRelease{}
	old.Status.ObservedResources = first.statuses
	old.Status.ObservedResources[0].LastObservedTime = &earlier

	t.Run("unchanged status and health keep the previous time", func(t *testing.T) {
		again := r.readObservedResources(ctx, planeClient, old, entries)
		if got := again.statuses[0].LastObservedTime; got == nil || !got.Equal(&earlier) {
			t.Errorf("LastObservedTime = %v, want %v", got, earlier)
		}
	})

	t.Run("a changed health moves it", func(t *testing.T) {
		changed := old.DeepCopy()
		changed.Status.ObservedResources[0].HealthStatus = openchoreov1alpha1.HealthStatusProgressing
		again := r.readObservedResources(ctx, planeClient, changed, entries)
		if got := again.statuses[0].LastObservedTime; got == nil || got.Equal(&earlier) {
			t.Errorf("LastObservedTime = %v, want a new time", got)
		}
	})

	t.Run("an entry for another object under the same id does not lend its time", func(t *testing.T) {
		other := old.DeepCopy()
		other.Status.ObservedResources[0].Namespace = "staging"
		again := r.readObservedResources(ctx, planeClient, other, entries)
		if got := again.statuses[0].LastObservedTime; got == nil || got.Equal(&earlier) {
			t.Errorf("LastObservedTime = %v, want a new time", got)
		}
	})
}

func TestNamesObject(t *testing.T) {
	gr := schema.GroupResource{Group: "apps", Resource: "deployments"}
	if !namesObject(apierrors.NewNotFound(gr, "api")) {
		t.Error("a missing object's NotFound names it")
	}
	unserved := &apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound, Code: 404}}
	if namesObject(unserved) {
		t.Error("a NotFound without details names nothing")
	}
	if namesObject(errors.New("plain")) {
		t.Error("an error that is not an API status names nothing")
	}
}

// ─────────────────────────────────────────────────────────────
// conditions
// ─────────────────────────────────────────────────────────────

func TestSetObservedConditions(t *testing.T) {
	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "Deployment", "prod", "api"),
		observedRes("db", "apps/v1", "StatefulSet", "prod", "db"),
		observedRes("cfg", "v1", "ConfigMap", "prod", "cfg"),
	})
	release := func() *openchoreov1alpha1.RenderedRelease {
		return &openchoreov1alpha1.RenderedRelease{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	}
	cond := func(rr *openchoreov1alpha1.RenderedRelease, ct controller.ConditionType) *metav1.Condition {
		return apimeta.FindStatusCondition(rr.Status.Conditions, string(ct))
	}

	t.Run("all read", func(t *testing.T) {
		rr := release()
		setObservedConditions(rr, entries, nil)
		c := cond(rr, ConditionObservedResourcesRead)
		if c == nil || c.Status != metav1.ConditionTrue || c.Reason != string(ReasonObservedReadSucceeded) {
			t.Fatalf("ObservedResourcesRead = %+v", c)
		}
		if c.ObservedGeneration != 5 {
			t.Errorf("observedGeneration = %d, want 5", c.ObservedGeneration)
		}
	})

	t.Run("failures with one reason use it", func(t *testing.T) {
		rr := release()
		setObservedConditions(rr, entries, []observedReadFailure{
			{entry: entries[0], reason: ReasonObservedResourceNotFound, detail: "not found"},
			{entry: entries[2], reason: ReasonObservedResourceNotFound, detail: "not found"},
		})
		c := cond(rr, ConditionObservedResourcesRead)
		if c.Status != metav1.ConditionFalse || c.Reason != string(ReasonObservedResourceNotFound) {
			t.Fatalf("ObservedResourcesRead = %+v", c)
		}
		for _, want := range []string{"2 of 3", "api (apps/v1 Deployment prod/api): not found", "cfg (v1 ConfigMap prod/cfg): not found"} {
			if !strings.Contains(c.Message, want) {
				t.Errorf("message %q does not contain %q", c.Message, want)
			}
		}
	})

	t.Run("failures with mixed reasons use ReadFailed and give each", func(t *testing.T) {
		rr := release()
		setObservedConditions(rr, entries, []observedReadFailure{
			{entry: entries[0], reason: ReasonObservedResourceNotFound, detail: "not found"},
			{entry: entries[1], reason: ReasonObservedReadForbidden, detail: "forbidden by the plane agent's RBAC"},
		})
		c := cond(rr, ConditionObservedResourcesRead)
		if c.Reason != string(ReasonObservedReadFailed) {
			t.Errorf("reason = %q, want ReadFailed", c.Reason)
		}
		if !strings.Contains(c.Message, "not found") || !strings.Contains(c.Message, "forbidden") {
			t.Errorf("message %q should give each failure's cause", c.Message)
		}
	})

	t.Run("a long list of failures is capped", func(t *testing.T) {
		many := make([]openchoreov1alpha1.ObservedResource, 15)
		for i := range many {
			name := fmt.Sprintf("cm-%d", i)
			many[i] = observedRes(name, "v1", "ConfigMap", "prod", name)
		}
		manyEntries := resolveObservedResources(many)
		failures := make([]observedReadFailure, 0, len(manyEntries))
		for _, e := range manyEntries {
			failures = append(failures, observedReadFailure{entry: e, reason: ReasonObservedResourceNotFound, detail: "not found"})
		}
		rr := release()
		setObservedConditions(rr, manyEntries, failures)
		msg := cond(rr, ConditionObservedResourcesRead).Message
		if !strings.Contains(msg, "15 of 15") || !strings.HasSuffix(msg, "; and 5 more") {
			t.Errorf("message = %q", msg)
		}
		if strings.Contains(msg, "cm-10 ") {
			t.Errorf("message names more than %d failures: %q", maxReportedReadFailures, msg)
		}
	})

	t.Run("no observed resources removes it", func(t *testing.T) {
		rr := release()
		setObservedConditions(rr, entries, nil)
		setObservedConditions(rr, nil, nil)
		if len(rr.Status.Conditions) != 0 {
			t.Errorf("conditions = %+v, want none", rr.Status.Conditions)
		}
	})
}

// ─────────────────────────────────────────────────────────────
// never written
// ─────────────────────────────────────────────────────────────

func TestExcludeObserved(t *testing.T) {
	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "Deployment", "prod", "api"),
	})
	observedDeploy := unstructuredObj("apps/v1", "Deployment", "prod", "api")
	// The same object listed under another version is still the observed one.
	observedOtherVersion := unstructuredObj("apps/v1beta2", "Deployment", "prod", "api")
	otherNamespace := unstructuredObj("apps/v1", "Deployment", "staging", "api")
	otherKind := unstructuredObj("v1", "Service", "prod", "api")

	kept := excludeObserved([]*unstructured.Unstructured{observedDeploy, observedOtherVersion, otherNamespace, otherKind}, entries)
	if len(kept) != 2 || kept[0] != otherNamespace || kept[1] != otherKind {
		t.Errorf("kept %d objects, want only the two that are not observed", len(kept))
	}
	if got := excludeObserved([]*unstructured.Unstructured{observedDeploy}, nil); len(got) != 1 {
		t.Error("with nothing observed, everything is kept")
	}
}

func TestCheckObservedOverlap(t *testing.T) {
	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("team-api", "apps/v1", "Deployment", "prod", "api"),
	})
	rendered := unstructuredObj("apps/v1", "Deployment", "prod", "api")
	rendered.SetLabels(map[string]string{"openchoreo.dev/rendered-release-resource-id": "deployment"})

	err := checkObservedOverlap([]*unstructured.Unstructured{rendered}, entries)
	if err == nil || !strings.Contains(err.Error(), "team-api") || !strings.Contains(err.Error(), "never written") {
		t.Errorf("err = %v, want one naming the observed resource", err)
	}
	if err := checkObservedOverlap([]*unstructured.Unstructured{unstructuredObj("apps/v1", "Deployment", "prod", "web")}, entries); err != nil {
		t.Errorf("distinct objects overlap: %v", err)
	}
	if err := checkObservedOverlap([]*unstructured.Unstructured{rendered}, nil); err != nil {
		t.Errorf("nothing observed, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────
// mode and requeue inputs
// ─────────────────────────────────────────────────────────────

func TestEffectiveMode(t *testing.T) {
	rr := &openchoreov1alpha1.RenderedRelease{}
	if got := effectiveMode(rr); got != openchoreov1alpha1.RenderedReleaseModeManage {
		t.Errorf("unset mode = %q, want Manage", got)
	}
	rr.Spec.Mode = openchoreov1alpha1.RenderedReleaseModeObserve
	if got := effectiveMode(rr); got != openchoreov1alpha1.RenderedReleaseModeObserve {
		t.Errorf("mode = %q, want Observe", got)
	}
}

func TestHasResurrectableLiveWorkload(t *testing.T) {
	scaledToZero := healthyDeployment(1, 1)
	if err := unstructured.SetNestedField(scaledToZero.Object, int64(0), "spec", "replicas"); err != nil {
		t.Fatal(err)
	}
	paused := scaledToZero.DeepCopy()
	if err := unstructured.SetNestedField(paused.Object, true, "spec", "paused"); err != nil {
		t.Fatal(err)
	}
	configMap := unstructuredObj("v1", "ConfigMap", "prod", "cfg")
	if err := unstructured.SetNestedField(configMap.Object, int64(0), "spec", "replicas"); err != nil {
		t.Fatal(err)
	}

	if !hasResurrectableLiveWorkload([]*unstructured.Unstructured{configMap, scaledToZero}) {
		t.Error("a Deployment at zero replicas is resurrectable")
	}
	if hasResurrectableLiveWorkload([]*unstructured.Unstructured{healthyDeployment(1, 1), paused, configMap}) {
		t.Error("running, paused and non-workload objects are not resurrectable")
	}
}
