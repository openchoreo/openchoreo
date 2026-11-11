// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
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

// planeServing returns a plane client whose every Get answers with served, as
// the server would send it.
func planeServing(served *unstructured.Unstructured) client.Client {
	return fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			obj.(*unstructured.Unstructured).Object = served.DeepCopy().Object
			return nil
		},
	}).Build()
}

func TestReadObservedResourcesJudgesHealthByServedKind(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	// generation 3 not yet observed: the Deployment check reports Progressing,
	// while the presence-only check would report Healthy.
	rollingOut := healthyDeployment(3, 2)

	t.Run("the kind the server returns wins over the entry's spelling", func(t *testing.T) {
		entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
			observedRes("api", "apps/v1", "DEPLOYMENT", "prod", "api")})
		result := r.readObservedResources(ctx, planeServing(rollingOut), &openchoreov1alpha1.RenderedRelease{}, entries)
		s := result.statuses[0]
		if s.HealthStatus != openchoreov1alpha1.HealthStatusProgressing {
			t.Errorf("health = %q, want Progressing from the Deployment check", s.HealthStatus)
		}
		if s.Kind != "DEPLOYMENT" {
			t.Errorf("status kind = %q, want the entry's spelling", s.Kind)
		}
	})

	t.Run("a response without a kind falls back to the entry's", func(t *testing.T) {
		kindless := rollingOut.DeepCopy()
		delete(kindless.Object, "kind")
		delete(kindless.Object, "apiVersion")
		entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
			observedRes("api", "apps/v1", "Deployment", "prod", "api")})
		result := r.readObservedResources(ctx, planeServing(kindless), &openchoreov1alpha1.RenderedRelease{}, entries)
		if got := result.statuses[0].HealthStatus; got != openchoreov1alpha1.HealthStatusProgressing {
			t.Errorf("health = %q, want Progressing from the Deployment check", got)
		}
	})
}

func TestReadObservedResourcesKeepsLastObservationOnPassingErrors(t *testing.T) {
	ctx := context.Background()
	r := &Reconciler{}
	deploy := healthyDeployment(2, 2)
	deploy.SetNamespace("prod")
	deploy.SetName("api")
	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "Deployment", "prod", "api")})

	first := r.readObservedResources(ctx, readOnlyPlaneClient(t, nil, deploy), &openchoreov1alpha1.RenderedRelease{}, entries)
	earlier := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
	old := &openchoreov1alpha1.RenderedRelease{}
	old.Status.ObservedResources = first.statuses
	old.Status.ObservedResources[0].LastObservedTime = &earlier

	t.Run("a generic error keeps the previous entry", func(t *testing.T) {
		blip := readOnlyPlaneClient(t, func(client.ObjectKey, client.Object) error {
			return errors.New("proxy request failed: 502 bad gateway")
		}, deploy)
		result := r.readObservedResources(ctx, blip, old, entries)
		if len(result.failures) != 1 || result.failures[0].reason != ReasonObservedReadFailed {
			t.Fatalf("failures = %+v, want one ReadFailed", result.failures)
		}
		s := result.statuses[0]
		if s.HealthStatus != openchoreov1alpha1.HealthStatusHealthy || s.Status == nil {
			t.Errorf("status = %+v, want the previous Healthy entry with its status", s)
		}
		if s.LastObservedTime == nil || !s.LastObservedTime.Equal(&earlier) {
			t.Errorf("LastObservedTime = %v, want the previous %v", s.LastObservedTime, earlier)
		}
	})

	t.Run("an object that is gone is Unknown, not carried forward", func(t *testing.T) {
		result := r.readObservedResources(ctx, readOnlyPlaneClient(t, nil), old, entries)
		if len(result.failures) != 1 || result.failures[0].reason != ReasonObservedResourceNotFound {
			t.Fatalf("failures = %+v, want one ResourceNotFound", result.failures)
		}
		s := result.statuses[0]
		if s.HealthStatus != openchoreov1alpha1.HealthStatusUnknown || s.Status != nil || s.LastObservedTime != nil {
			t.Errorf("status = %+v, want Unknown with no status or time", s)
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

// TestReadObservedResourcesThroughProxyClient reads through ProxyClient, the plane
// client production uses, against a server answering as the gateway would. The
// envtest client checks kinds with discovery before sending anything, and the
// other unit tests return errors built by hand, so this is the test that covers
// how ProxyClient turns the plane's answers into the errors classified here.
func TestReadObservedResourcesThroughProxyClient(t *testing.T) {
	rollingOut := healthyDeployment(3, 2)
	rollingOut.SetNamespace("prod")
	rollingOut.SetName("api")

	writeJSON := func(w http.ResponseWriter, code int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}
	notFound := func(details *metav1.StatusDetails) metav1.Status {
		return metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotFound,
			Code: http.StatusNotFound, Details: details}
	}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The plane's API path follows /k8s in the proxy URL.
		_, path, _ := strings.Cut(r.URL.Path, "/k8s")
		switch path {
		case "/apis/apps/v1/namespaces/prod/deployments/api":
			writeJSON(w, http.StatusOK, rollingOut.Object)
		case "/apis/apps/v1/namespaces/prod/deployments/gone":
			writeJSON(w, http.StatusNotFound, notFound(&metav1.StatusDetails{Name: "gone", Group: "apps", Kind: "deployments"}))
		case "/apis/apps/v1beta1/namespaces/prod/deployments/api", // a version the plane does not serve
			"/apis/apps/v1/deployments/api": // a namespaced kind asked for without a namespace
			writeJSON(w, http.StatusNotFound, notFound(&metav1.StatusDetails{}))
		case "/apis/autoscaling.k8s.io/v1/namespaces/prod/verticalpodautoscalers/api":
			// A group the plane does not serve: the API server answers in plain text.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 page not found"))
		case "/apis/storage.k8s.io/v1/storageclasses/local-path":
			writeJSON(w, http.StatusForbidden, metav1.Status{Status: metav1.StatusFailure,
				Reason: metav1.StatusReasonForbidden, Code: http.StatusForbidden})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer gateway.Close()

	plane, err := kubernetesClient.NewProxyClient(gateway.URL, "dataplane/test", "_cluster", "cdp", nil)
	if err != nil {
		t.Fatalf("NewProxyClient: %v", err)
	}
	entries := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "Deployment", "prod", "api"),
		observedRes("gone", "apps/v1", "Deployment", "prod", "gone"),
		observedRes("old-version", "apps/v1beta1", "Deployment", "prod", "api"),
		observedRes("no-namespace", "apps/v1", "Deployment", "", "api"),
		observedRes("vpa", "autoscaling.k8s.io/v1", "VerticalPodAutoscaler", "prod", "api"),
		observedRes("storageclass", "storage.k8s.io/v1", "StorageClass", "", "local-path"),
	})
	result := (&Reconciler{}).readObservedResources(context.Background(), plane, &openchoreov1alpha1.RenderedRelease{}, entries)

	if got := result.statuses[0].HealthStatus; got != openchoreov1alpha1.HealthStatusProgressing {
		t.Errorf("api health = %q, want Progressing from the Deployment check", got)
	}
	failures := make(map[string]observedReadFailure, len(result.failures))
	for _, f := range result.failures {
		failures[f.entry.id] = f
	}
	for id, want := range map[string]controller.ConditionReason{
		"gone":         ReasonObservedResourceNotFound,
		"old-version":  ReasonObservedKindNotFound,
		"no-namespace": ReasonObservedKindNotFound,
		// Known limitation: ProxyClient turns the plain-text 404 for an unserved
		// group into a not-found that names the object.
		"vpa":          ReasonObservedResourceNotFound,
		"storageclass": ReasonObservedReadForbidden,
	} {
		f, ok := failures[id]
		if !ok {
			t.Errorf("%s: no failure recorded, want %s", id, want)
			continue
		}
		if f.reason != want {
			t.Errorf("%s: reason = %s, want %s (detail %q)", id, f.reason, want, f.detail)
		}
	}
	for _, id := range []string{"old-version", "no-namespace"} {
		if d := failures[id].detail; !strings.Contains(d, "namespace does not fit the kind's scope") {
			t.Errorf("%s: detail = %q, want it to mention the scope", id, d)
		}
	}
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

	// The kind is compared case-insensitively, as ProxyClient builds the request
	// path from it: an entry spelled DEPLOYMENT reads, and protects, the Deployment.
	upperCase := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("api", "apps/v1", "DEPLOYMENT", "prod", "api"),
	})
	if got := excludeObserved([]*unstructured.Unstructured{observedDeploy}, upperCase); len(got) != 0 {
		t.Error("an entry spelled DEPLOYMENT must keep the Deployment out of the deletes")
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
	if err := checkObservedOverlap([]*unstructured.Unstructured{unstructuredObj("apps/v1beta2", "Deployment", "prod", "api")}, entries); err == nil {
		t.Error("the same object under another API version overlaps too")
	}
	if err := checkObservedOverlap([]*unstructured.Unstructured{rendered}, nil); err != nil {
		t.Errorf("nothing observed, got %v", err)
	}

	upperCase := resolveObservedResources([]openchoreov1alpha1.ObservedResource{
		observedRes("team-api", "apps/v1", "DEPLOYMENT", "prod", "api"),
	})
	err = checkObservedOverlap([]*unstructured.Unstructured{rendered}, upperCase)
	if err == nil {
		t.Fatal("an entry spelled DEPLOYMENT overlaps the rendered Deployment")
	}
	if !strings.Contains(err.Error(), "(Deployment.apps prod/api)") {
		t.Errorf("err = %v, want the object named as rendered", err)
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
