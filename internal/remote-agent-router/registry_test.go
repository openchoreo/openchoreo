// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remoteagentrouter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testSNI       = "dp-doclet-development.remote-connect"
	testAgentName = "openchoreo-remote-agent"
	testBackend   = "openchoreo-remote-agent.dp-doclet-development.svc.cluster.local:8443"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// agentService builds a remote-agent Service as the control-plane provisioner creates it:
// labeled managed-by and annotated with the SNI the router routes on.
func agentService(ns, sni string, port int32) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testAgentName,
			Namespace: ns,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "openchoreo-api-remote-connect"},
		},
	}
	if sni != "" {
		svc.Annotations = map[string]string{DefaultSNIAnnotationKey: sni}
	}
	if port != 0 {
		svc.Spec.Ports = []corev1.ServicePort{{Port: port}}
	}
	return svc
}

func newTestRegistry(objs ...runtime.Object) (*registry, *k8sfake.Clientset) {
	cs := k8sfake.NewSimpleClientset(objs...)
	return newRegistry(cs, DefaultLabelSelector, DefaultSNIAnnotationKey, testLogger()), cs
}

// byHostSnapshot reads the backend map without going through lookup (which would
// refresh on a miss and mask what a refresh actually produced).
func (r *registry) byHostSnapshot() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.byHost))
	maps.Copy(out, r.byHost)
	return out
}

// TestRegistryRefreshBuildsBackendMap: an annotated, labeled Service with a port becomes
// a routable backend; anything missing one of those is skipped, not mapped to a bogus address.
func TestRegistryRefreshBuildsBackendMap(t *testing.T) {
	unlabeled := agentService("dp-other-development", "unlabeled.remote-connect", 8443)
	unlabeled.Labels = nil

	reg, _ := newTestRegistry(
		agentService("dp-doclet-development", testSNI, 8443),
		agentService("dp-noanno-development", "", 8443),                   // no SNI annotation
		agentService("dp-noport-development", "noport.remote-connect", 0), // no ports
		unlabeled, // not managed by remote-connect
		agentService("dp-second-development", "second.remote-connect", 9443), // a second real agent
	)

	reg.refresh(context.Background())
	got := reg.byHostSnapshot()

	if got[testSNI] != testBackend {
		t.Errorf("byHost[%q] = %q, want %q", testSNI, got[testSNI], testBackend)
	}
	if want := "openchoreo-remote-agent.dp-second-development.svc.cluster.local:9443"; got["second.remote-connect"] != want {
		t.Errorf("byHost[second.remote-connect] = %q, want %q", got["second.remote-connect"], want)
	}
	if len(got) != 2 {
		t.Errorf("expected only the 2 annotated+labeled+ported agents to be routable, got %v", got)
	}
	if _, ok := got["unlabeled.remote-connect"]; ok {
		t.Error("a Service without the managed-by label must not be routable")
	}
	if _, ok := got["noport.remote-connect"]; ok {
		t.Error("a Service with no ports must not be routable")
	}
}

// TestRegistryLookupRefreshesOnMiss proves a just-provisioned agent becomes routable
// on the very next connection instead of waiting out a refresh interval.
func TestRegistryLookupRefreshesOnMiss(t *testing.T) {
	reg, _ := newTestRegistry(agentService("dp-doclet-development", testSNI, 8443))

	// No refresh has run yet, so the map is empty — lookup must refresh and find it.
	if len(reg.byHostSnapshot()) != 0 {
		t.Fatal("precondition: registry should start empty")
	}
	addr, ok := reg.lookup(context.Background(), testSNI)
	if !ok || addr != testBackend {
		t.Fatalf("lookup on miss = (%q, %v), want (%q, true)", addr, ok, testBackend)
	}
}

func TestRegistryLookupUnknownSNI(t *testing.T) {
	reg, _ := newTestRegistry(agentService("dp-doclet-development", testSNI, 8443))
	if addr, ok := reg.lookup(context.Background(), "nobody.remote-connect"); ok {
		t.Fatalf("expected miss for an unknown SNI, got %q", addr)
	}
}

// TestRegistryRefreshKeepsMapOnListError: an API error must leave the previous backends
// in place — clearing the map would black-hole every live session.
func TestRegistryRefreshKeepsMapOnListError(t *testing.T) {
	reg, cs := newTestRegistry(agentService("dp-doclet-development", testSNI, 8443))
	reg.refresh(context.Background())
	if len(reg.byHostSnapshot()) != 1 {
		t.Fatal("precondition: expected one backend after the first refresh")
	}

	cs.PrependReactor("list", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api server unavailable")
	})
	reg.refresh(context.Background())

	if got := reg.byHostSnapshot(); got[testSNI] != testBackend {
		t.Errorf("backends were dropped on a list error: %v", got)
	}
}

// TestRegistryStartRefreshesPeriodically covers the discovery loop picking up an agent
// provisioned after start-up.
func TestRegistryStartRefreshesPeriodically(t *testing.T) {
	reg, cs := newTestRegistry()
	ctx := t.Context()

	go reg.Start(ctx, 10*time.Millisecond)

	if _, err := cs.CoreV1().Services("dp-doclet-development").
		Create(ctx, agentService("dp-doclet-development", testSNI, 8443), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if reg.byHostSnapshot()[testSNI] == testBackend {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("agent never became routable via the refresh loop: %v", reg.byHostSnapshot())
}

// TestRegistryDuplicateSNIIsDeterministic: one SNI resolves to one backend, and the
// same one across refreshes.
func TestRegistryDuplicateSNIIsDeterministic(t *testing.T) {
	reg, _ := newTestRegistry(
		agentService("dp-a-development", "shared.remote-connect", 8443),
		agentService("dp-b-development", "shared.remote-connect", 8443),
	)

	reg.refresh(context.Background())
	first := reg.byHostSnapshot()["shared.remote-connect"]
	if first == "" {
		t.Fatal("duplicate SNI dropped the backend entirely")
	}
	if got := len(reg.byHostSnapshot()); got != 1 {
		t.Fatalf("expected one backend for one SNI, got %d", got)
	}

	for range 5 {
		reg.refresh(context.Background())
		if got := reg.byHostSnapshot()["shared.remote-connect"]; got != first {
			t.Fatalf("backend flipped across refreshes: %q then %q", first, got)
		}
	}
}

// countingRegistry returns a registry whose List calls are counted.
func countingRegistry(objs ...runtime.Object) (*registry, *int32) {
	var lists int32
	cs := k8sfake.NewSimpleClientset(objs...)
	cs.PrependReactor("list", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		atomic.AddInt32(&lists, 1)
		return false, nil, nil // fall through to the tracker
	})
	return newRegistry(cs, DefaultLabelSelector, DefaultSNIAnnotationKey, testLogger()), &lists
}

// TestRegistryMissRefreshIsRateLimited: unknown SNIs arrive before any authentication, so
// a burst of them must not produce a List per lookup.
func TestRegistryMissRefreshIsRateLimited(t *testing.T) {
	reg, lists := countingRegistry(agentService("dp-doclet-development", testSNI, 8443))

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.lookup(context.Background(), "nobody.remote-connect")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(lists); got > 2 {
		t.Fatalf("50 concurrent misses caused %d List calls, want at most 2", got)
	}
}

// TestRegistryMissRefreshResumesAfterCooldown: rate limiting must not stop a newly
// provisioned agent from becoming routable.
func TestRegistryMissRefreshResumesAfterCooldown(t *testing.T) {
	reg, _ := newTestRegistry()

	reg.lookup(context.Background(), testSNI) // primes lastMissRefresh
	reg.missMu.Lock()
	reg.lastMissRefresh = time.Now().Add(-2 * missRefreshCooldown)
	reg.missMu.Unlock()

	cs := k8sfake.NewSimpleClientset(agentService("dp-doclet-development", testSNI, 8443))
	reg.client = cs

	addr, ok := reg.lookup(context.Background(), testSNI)
	if !ok || addr != testBackend {
		t.Fatalf("lookup after cooldown = (%q, %v), want (%q, true)", addr, ok, testBackend)
	}
}
