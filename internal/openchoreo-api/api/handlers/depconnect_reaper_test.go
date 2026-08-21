// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// reaperNow is the fixed "current time" every reaper test evaluates the TTL against.
var reaperNow = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

const reaperTTLSeconds = 1800 // 30 minutes idle

// fakeDataPlaneClientProvider hands out a per-plane client, or an error for planes
// listed in failFor (an unreachable data plane).
type fakeDataPlaneClientProvider struct {
	clients map[string]client.Client
	failFor map[string]bool
}

func (p *fakeDataPlaneClientProvider) DataPlaneClient(dp *openchoreov1alpha1.DataPlane) (client.Client, error) {
	return p.clientFor(dp.Name)
}

func (p *fakeDataPlaneClientProvider) ClusterDataPlaneClient(cdp *openchoreov1alpha1.ClusterDataPlane) (client.Client, error) {
	return p.clientFor(cdp.Name)
}

func (p *fakeDataPlaneClientProvider) clientFor(name string) (client.Client, error) {
	if p.failFor[name] {
		return nil, errors.New("data plane unreachable")
	}
	c, ok := p.clients[name]
	if !ok {
		return nil, errors.New("no client for plane " + name)
	}
	return c, nil
}

// depAgentObjects builds the trio the provisioner creates for one agent namespace. A
// lastUsed of "" omits the annotation entirely.
func depAgentObjects(ns, lastUsed string, managed bool) []client.Object {
	labels := map[string]string{}
	if managed {
		labels["app.kubernetes.io/managed-by"] = managedByLabelValue
	}
	annotations := map[string]string{}
	if lastUsed != "" {
		annotations[lastUsedAnnotation] = lastUsed
	}
	meta := metav1.ObjectMeta{
		Name:        depAgentName,
		Namespace:   ns,
		Labels:      labels,
		Annotations: annotations,
	}
	return []client.Object{
		&appsv1.Deployment{ObjectMeta: meta},
		&corev1.Service{ObjectMeta: *meta.DeepCopy()},
		&corev1.Secret{ObjectMeta: *meta.DeepCopy()},
	}
}

func rfc3339(t time.Time) string { return t.Format(time.RFC3339) }

// dataPlaneScheme carries both the data-plane workload types and the OpenChoreo types,
// so one scheme serves the control-plane client and the per-plane clients.
func dataPlaneScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme, appsv1.AddToScheme, openchoreov1alpha1.AddToScheme,
	} {
		if err := add(s); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func newDPClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(dataPlaneScheme(t)).WithObjects(objs...).Build()
}

// newReaper wires a reaper over the given control-plane objects and per-plane clients,
// pinned to reaperNow.
func newReaper(t *testing.T, cpObjs []client.Object, provider *fakeDataPlaneClientProvider) *DepAgentReaper {
	t.Helper()
	cp := fake.NewClientBuilder().WithScheme(dataPlaneScheme(t)).WithObjects(cpObjs...).Build()
	r := NewDepAgentReaper(cp, provider,
		config.DepConnectConfig{ReaperIntervalSeconds: 1, ReaperTTLSeconds: reaperTTLSeconds},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.now = func() time.Time { return reaperNow }
	return r
}

func reaperDataPlane(name string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
}

// assertAgentGone asserts the whole trio was deleted from ns.
func assertAgentGone(t *testing.T, c client.Client, ns string) {
	t.Helper()
	for name, obj := range map[string]client.Object{
		"deployment": &appsv1.Deployment{},
		"service":    &corev1.Service{},
		"secret":     &corev1.Secret{},
	} {
		err := c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: depAgentName}, obj)
		if !apierrors.IsNotFound(err) {
			t.Errorf("%s in %s: expected NotFound after reaping, got %v", name, ns, err)
		}
	}
}

// assertAgentAlive asserts the whole trio survived in ns.
func assertAgentAlive(t *testing.T, c client.Client, ns string) {
	t.Helper()
	for name, obj := range map[string]client.Object{
		"deployment": &appsv1.Deployment{},
		"service":    &corev1.Service{},
		"secret":     &corev1.Secret{},
	} {
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: depAgentName}, obj); err != nil {
			t.Errorf("%s in %s: expected it to survive, got %v", name, ns, err)
		}
	}
}

// TestReaperDeletesIdleAgentsOnly: an agent idle past the TTL is torn down completely,
// one touched within it is left alone. Getting this wrong kills live sessions.
func TestReaperDeletesIdleAgentsOnly(t *testing.T) {
	const staleNS, freshNS = "dp-default-stale-development", "dp-default-fresh-development"
	objs := append(
		depAgentObjects(staleNS, rfc3339(reaperNow.Add(-45*time.Minute)), true),
		depAgentObjects(freshNS, rfc3339(reaperNow.Add(-5*time.Minute)), true)...,
	)
	dpClient := newDPClient(t, objs...)
	r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
		&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

	r.reapOnce(context.Background())

	assertAgentGone(t, dpClient, staleNS)
	assertAgentAlive(t, dpClient, freshNS)
}

// TestReaperTTLBoundary: an agent touched exactly at the TTL edge is stale (the check is
// "newer than cutoff survives"), one a second inside it survives.
func TestReaperTTLBoundary(t *testing.T) {
	const atEdgeNS, insideNS = "dp-default-edge-development", "dp-default-inside-development"
	ttl := time.Duration(reaperTTLSeconds) * time.Second
	objs := append(
		depAgentObjects(atEdgeNS, rfc3339(reaperNow.Add(-ttl)), true),
		depAgentObjects(insideNS, rfc3339(reaperNow.Add(-ttl+time.Second)), true)...,
	)
	dpClient := newDPClient(t, objs...)
	r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
		&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

	r.reapOnce(context.Background())

	assertAgentGone(t, dpClient, atEdgeNS)
	assertAgentAlive(t, dpClient, insideNS)
}

// TestReaperReapsUndatedAgents: a missing or unparseable last-used stamp counts as stale,
// so a half-provisioned agent cannot leak forever.
func TestReaperReapsUndatedAgents(t *testing.T) {
	tests := []struct {
		name     string
		lastUsed string
	}{
		{"missing annotation", ""},
		{"unparseable annotation", "not-a-timestamp"},
		{"empty-ish annotation", " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const ns = "dp-default-undated-development"
			dpClient := newDPClient(t, depAgentObjects(ns, tt.lastUsed, true)...)
			r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
				&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

			r.reapOnce(context.Background())

			assertAgentGone(t, dpClient, ns)
		})
	}
}

// TestReaperIgnoresUnmanagedDeployments: deletion is by fixed name, so the reaper must
// only ever act on resources it owns.
func TestReaperIgnoresUnmanagedDeployments(t *testing.T) {
	const ns = "someone-elses-namespace"
	// Same name, stale stamp, but not labeled managed-by dep-connect.
	dpClient := newDPClient(t, depAgentObjects(ns, rfc3339(reaperNow.Add(-24*time.Hour)), false)...)
	r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
		&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

	r.reapOnce(context.Background())

	assertAgentAlive(t, dpClient, ns)
}

// TestReaperSweepsDataPlanesAndClusterDataPlanes: a sweep must cover both plane kinds.
func TestReaperSweepsDataPlanesAndClusterDataPlanes(t *testing.T) {
	const nsA, nsB = "dp-default-a-development", "dp-default-b-development"
	stale := rfc3339(reaperNow.Add(-2 * time.Hour))
	clientA := newDPClient(t, depAgentObjects(nsA, stale, true)...)
	clientB := newDPClient(t, depAgentObjects(nsB, stale, true)...)

	cpObjs := []client.Object{
		reaperDataPlane("dp-1"),
		&openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "cdp-1"}},
	}
	r := newReaper(t, cpObjs, &fakeDataPlaneClientProvider{clients: map[string]client.Client{
		"dp-1": clientA, "cdp-1": clientB,
	}})

	r.reapOnce(context.Background())

	assertAgentGone(t, clientA, nsA)
	assertAgentGone(t, clientB, nsB)
}

// TestReaperContinuesWhenAPlaneIsUnreachable: one broken plane must not stop the others.
func TestReaperContinuesWhenAPlaneIsUnreachable(t *testing.T) {
	const healthyNS = "dp-default-healthy-development"
	healthy := newDPClient(t, depAgentObjects(healthyNS, rfc3339(reaperNow.Add(-2*time.Hour)), true)...)

	cpObjs := []client.Object{reaperDataPlane("broken"), reaperDataPlane("healthy")}
	r := newReaper(t, cpObjs, &fakeDataPlaneClientProvider{
		clients: map[string]client.Client{"healthy": healthy},
		failFor: map[string]bool{"broken": true},
	})

	r.reapOnce(context.Background())

	assertAgentGone(t, healthy, healthyNS)
}

// TestReaperHandlesAlreadyDeletedResources: a partially-deleted agent still reaps cleanly.
func TestReaperHandlesAlreadyDeletedResources(t *testing.T) {
	const ns = "dp-default-partial-development"
	objs := depAgentObjects(ns, rfc3339(reaperNow.Add(-2*time.Hour)), true)
	dpClient := newDPClient(t, objs[0]) // Deployment only
	r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
		&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

	r.reapOnce(context.Background())

	assertAgentGone(t, dpClient, ns)
}

// TestReaperStartReapsOnTickAndStopsOnCancel covers the loop itself.
func TestReaperStartReapsOnTickAndStopsOnCancel(t *testing.T) {
	const ns = "dp-default-loop-development"
	dpClient := newDPClient(t, depAgentObjects(ns, rfc3339(reaperNow.Add(-2*time.Hour)), true)...)
	r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
		&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Start(ctx); close(done) }()

	deadline := time.Now().Add(10 * time.Second)
	reaped := false
	for time.Now().Before(deadline) {
		err := dpClient.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: depAgentName}, &appsv1.Deployment{})
		if apierrors.IsNotFound(err) {
			reaped = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !reaped {
		cancel()
		t.Fatal("the reap loop never deleted the idle agent")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

// TestReaperSkipsAgentRefreshedAfterListing: an agent touched between the list and the
// delete is still in use.
func TestReaperSkipsAgentRefreshedAfterListing(t *testing.T) {
	const ns = "dp-default-raced-development"
	stale := rfc3339(reaperNow.Add(-2 * time.Hour))
	fresh := rfc3339(reaperNow.Add(-time.Minute))

	// Refresh the annotation on the re-read, standing in for a resolve landing between
	// the list and the delete.
	dpClient := fake.NewClientBuilder().WithScheme(dataPlaneScheme(t)).
		WithObjects(depAgentObjects(ns, stale, true)...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := c.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				if dep, isDep := obj.(*appsv1.Deployment); isDep {
					dep.Annotations[lastUsedAnnotation] = fresh
				}
				return nil
			},
		}).Build()
	r := newReaper(t, []client.Object{reaperDataPlane("dp-1")},
		&fakeDataPlaneClientProvider{clients: map[string]client.Client{"dp-1": dpClient}})

	r.reapOnce(context.Background())

	assertAgentAlive(t, dpClient, ns)
}
