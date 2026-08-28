// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fabric

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	fcache "k8s.io/client-go/tools/cache/testing"
)

func testDiscovery(selfPod string) *EndpointSliceDiscovery {
	return NewEndpointSliceDiscovery(nil, "openchoreo-control-plane", "cluster-gateway-mesh", 8445, selfPod, testLogger())
}

// endpointSlice builds a slice whose endpoints are (podName, ip, ready)
// triples. A nil ready means the condition is absent, which Kubernetes treats
// as ready.
func endpointSlice(name string, endpoints ...discoveryv1.Endpoint) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "openchoreo-control-plane"},
		Endpoints:  endpoints,
	}
}

func endpoint(podName, ip string, ready *bool) discoveryv1.Endpoint {
	ep := discoveryv1.Endpoint{
		Addresses:  []string{ip},
		Conditions: discoveryv1.EndpointConditions{Ready: ready},
	}
	if podName != "" {
		ep.TargetRef = &corev1.ObjectReference{Name: podName}
	}
	return ep
}

func boolPtr(b bool) *bool { return &b }

func storeWith(t *testing.T, objs ...any) cache.Store {
	t.Helper()
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)
	for _, o := range objs {
		if err := store.Add(o); err != nil {
			t.Fatalf("failed to seed store: %v", err)
		}
	}
	return store
}

// The peer set decides where agent traffic is routed, so every filter here
// matters: routing to a not-ready pod, or to ourselves, sends requests
// somewhere that cannot serve them.
func TestPeersFromStore(t *testing.T) {
	tests := []struct {
		name string
		self string
		objs []any
		want []Peer
	}{
		{
			name: "ready peers are reported, sorted by ID",
			self: "gw-self",
			objs: []any{endpointSlice("s1",
				endpoint("gw-c", "10.0.0.3", boolPtr(true)),
				endpoint("gw-a", "10.0.0.1", boolPtr(true)),
			)},
			want: []Peer{
				{ID: "gw-a", Addr: "10.0.0.1:8445"},
				{ID: "gw-c", Addr: "10.0.0.3:8445"},
			},
		},
		{
			name: "this pod is excluded",
			self: "gw-self",
			objs: []any{endpointSlice("s1",
				endpoint("gw-self", "10.0.0.9", boolPtr(true)),
				endpoint("gw-a", "10.0.0.1", boolPtr(true)),
			)},
			want: []Peer{{ID: "gw-a", Addr: "10.0.0.1:8445"}},
		},
		{
			name: "not-ready endpoints are skipped",
			self: "gw-self",
			objs: []any{endpointSlice("s1",
				endpoint("gw-a", "10.0.0.1", boolPtr(false)),
				endpoint("gw-b", "10.0.0.2", boolPtr(true)),
			)},
			want: []Peer{{ID: "gw-b", Addr: "10.0.0.2:8445"}},
		},
		{
			name: "an absent ready condition counts as ready",
			self: "gw-self",
			objs: []any{endpointSlice("s1", endpoint("gw-a", "10.0.0.1", nil))},
			want: []Peer{{ID: "gw-a", Addr: "10.0.0.1:8445"}},
		},
		{
			name: "endpoints without an address are skipped",
			self: "gw-self",
			objs: []any{endpointSlice("s1",
				discoveryv1.Endpoint{Conditions: discoveryv1.EndpointConditions{Ready: boolPtr(true)}},
				endpoint("gw-a", "10.0.0.1", boolPtr(true)),
			)},
			want: []Peer{{ID: "gw-a", Addr: "10.0.0.1:8445"}},
		},
		{
			name: "without a targetRef the address identifies the peer",
			self: "gw-self",
			objs: []any{endpointSlice("s1", endpoint("", "10.0.0.1", boolPtr(true)))},
			want: []Peer{{ID: "10.0.0.1", Addr: "10.0.0.1:8445"}},
		},
		{
			name: "endpoints are merged across slices and deduplicated",
			self: "gw-self",
			objs: []any{
				endpointSlice("s1", endpoint("gw-a", "10.0.0.1", boolPtr(true))),
				endpointSlice("s2", endpoint("gw-a", "10.0.0.1", boolPtr(true))),
				endpointSlice("s3", endpoint("gw-b", "10.0.0.2", boolPtr(true))),
			},
			want: []Peer{
				{ID: "gw-a", Addr: "10.0.0.1:8445"},
				{ID: "gw-b", Addr: "10.0.0.2:8445"},
			},
		},
		{
			name: "objects of other kinds are ignored",
			self: "gw-self",
			objs: []any{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "openchoreo-control-plane"}},
				endpointSlice("s1", endpoint("gw-a", "10.0.0.1", boolPtr(true))),
			},
			want: []Peer{{ID: "gw-a", Addr: "10.0.0.1:8445"}},
		},
		{
			name: "a lone self endpoint yields no peers",
			self: "gw-self",
			objs: []any{endpointSlice("s1", endpoint("gw-self", "10.0.0.9", boolPtr(true)))},
			want: []Peer{},
		},
		{
			name: "an empty store yields no peers",
			self: "gw-self",
			objs: nil,
			want: []Peer{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testDiscovery(tt.self)
			got := d.peersFromStore(storeWith(t, tt.objs...))

			if len(got) != len(tt.want) {
				t.Fatalf("got %d peers %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("peer %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The fingerprint is what suppresses duplicate emissions, so it must change
// when — and only when — the routable peer set changes.
func TestPeerFingerprint(t *testing.T) {
	a := []Peer{{ID: "gw-a", Addr: "10.0.0.1:8445"}}
	b := []Peer{{ID: "gw-b", Addr: "10.0.0.2:8445"}}
	aMoved := []Peer{{ID: "gw-a", Addr: "10.0.0.9:8445"}}

	// Stability across separately built slices with the same contents: the
	// dedup only works if an unchanged peer set fingerprints identically.
	aAgain := []Peer{{ID: "gw-a", Addr: "10.0.0.1:8445"}}
	if peerFingerprint(a) != peerFingerprint(aAgain) {
		t.Fatal("fingerprint must be stable for an equivalent peer set")
	}
	if peerFingerprint(a) == peerFingerprint(b) {
		t.Fatal("different peers must fingerprint differently")
	}
	if peerFingerprint(a) == peerFingerprint(aMoved) {
		t.Fatal("a peer that changed address must fingerprint differently")
	}
	if peerFingerprint(append(append([]Peer{}, a...), b...)) == peerFingerprint(a) {
		t.Fatal("adding a peer must fingerprint differently")
	}
	// The empty set fingerprints to the zero value, which is exactly why
	// "nothing emitted yet" cannot be represented by an empty fingerprint.
	if peerFingerprint(nil) != "" {
		t.Fatalf("empty peer set fingerprint = %q, want \"\"", peerFingerprint(nil))
	}
}

// A peerless replica must still report its (empty) peer set. The mesh holds a
// pod out of the Service until discovery reports at least once, so suppressing
// this emission strands a single-replica gateway unready for the whole
// convergence grace period.
func TestWatch_EmitsEmptyPeerSetOnFirstSync(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	t.Cleanup(source.Shutdown)

	d := testDiscovery("gw-self")
	d.newListerWatcher = func() cache.ListerWatcher { return source }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := d.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Only this pod is in the slice, so the computed peer set is empty.
	source.Add(endpointSlice("s1", endpoint("gw-self", "10.0.0.9", boolPtr(true))))

	select {
	case peers := <-ch:
		if len(peers) != 0 {
			t.Fatalf("expected an empty peer set, got %v", peers)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("discovery never reported: a peerless replica would stay unready")
	}
}

// Only changes are published: a re-sync that computes the same peer set must
// not wake the mesh into reconciling links it already holds.
func TestWatch_EmitsOnlyOnChange(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	t.Cleanup(source.Shutdown)

	d := testDiscovery("gw-self")
	d.newListerWatcher = func() cache.ListerWatcher { return source }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := d.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	slice := endpointSlice("s1", endpoint("gw-a", "10.0.0.1", boolPtr(true)))
	source.Add(slice)

	select {
	case peers := <-ch:
		if len(peers) != 1 || peers[0].ID != "gw-a" {
			t.Fatalf("first emission = %v, want [gw-a]", peers)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("discovery never reported the first peer set")
	}

	// Touch the object without changing the routable set.
	updated := endpointSlice("s1", endpoint("gw-a", "10.0.0.1", boolPtr(true)))
	updated.Labels = map[string]string{"unrelated": "change"}
	source.Modify(updated)

	select {
	case peers := <-ch:
		t.Fatalf("an unchanged peer set must not be re-emitted, got %v", peers)
	case <-time.After(300 * time.Millisecond):
	}

	// A real change must come through.
	source.Modify(endpointSlice("s1",
		endpoint("gw-a", "10.0.0.1", boolPtr(true)),
		endpoint("gw-b", "10.0.0.2", boolPtr(true)),
	))

	select {
	case peers := <-ch:
		if len(peers) != 2 {
			t.Fatalf("expected both peers after the change, got %v", peers)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a changed peer set was not reported")
	}
}

// Losing every peer is itself a change the mesh must hear about: it is how a
// replica learns its links are gone.
func TestWatch_ReportsPeerRemoval(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	t.Cleanup(source.Shutdown)

	d := testDiscovery("gw-self")
	d.newListerWatcher = func() cache.ListerWatcher { return source }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := d.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	slice := endpointSlice("s1", endpoint("gw-a", "10.0.0.1", boolPtr(true)))
	source.Add(slice)
	waitForPeers(t, ch, 1)

	source.Delete(slice)
	waitForPeers(t, ch, 0)
}

func waitForPeers(t *testing.T, ch <-chan []Peer, want int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case peers := <-ch:
			if len(peers) == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a peer set of size %d", want)
		}
	}
}

// A consumer that stops reading must not wedge the informer, and must not be
// handed a stale view once it returns. The channel keeps only the newest peer
// set, since an older one is never the right basis for reconciling links.
func TestWatch_SlowConsumerGetsTheLatestPeerSet(t *testing.T) {
	source := fcache.NewFakeControllerSource()
	t.Cleanup(source.Shutdown)

	d := testDiscovery("gw-self")
	d.newListerWatcher = func() cache.ListerWatcher { return source }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := d.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// publish makes the peer set exactly n endpoints wide.
	publish := func(n int) {
		eps := make([]discoveryv1.Endpoint, 0, n)
		for j := range n {
			eps = append(eps, endpoint(fmt.Sprintf("gw-%03d", j), fmt.Sprintf("10.0.0.%d", j+1), boolPtr(true)))
		}
		slice := endpointSlice("s1", eps...)
		if n == 1 {
			source.Add(slice)
		} else {
			source.Modify(slice)
		}
	}

	// One update at a time, each confirmed queued before the next: every emit
	// reads the store as it stands right then, so a burst of updates collapses
	// into a single peer set and would never fill the buffer.
	for n := 1; n <= cap(ch); n++ {
		publish(n)
		deadline := time.After(10 * time.Second)
		for len(ch) < n {
			select {
			case <-deadline:
				t.Fatalf("peer set of %d never queued (%d/%d slots filled)", n, len(ch), cap(ch))
			case <-time.After(5 * time.Millisecond):
			}
		}
	}

	// Nothing has been read, so this update can only reach the consumer by
	// displacing a queued one.
	newest := cap(ch) + 1
	publish(newest)

	// The buffer stays full while the newest set takes the place of the oldest.
	settle := time.After(2 * time.Second)
	for {
		if len(ch) < cap(ch) {
			t.Fatalf("the peer channel drained on its own to %d/%d", len(ch), cap(ch))
		}
		select {
		case <-settle:
		case <-time.After(10 * time.Millisecond):
			continue
		}
		break
	}

	// The oldest set must be the one that was dropped: a consumer that returns
	// to a queue still headed by the first peer set would reconcile its links
	// against a view several updates stale.
	select {
	case peers := <-ch:
		if len(peers) == 1 {
			t.Fatal("the oldest peer set was still queued: the newest was dropped instead of displacing it")
		}
	default:
		t.Fatal("the peer channel was empty")
	}

	// The producer must not have blocked: the final, largest peer set has to
	// arrive even though nothing was reading while it was produced.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case peers := <-ch:
			if len(peers) == newest {
				return
			}
		case <-deadline:
			t.Fatal("the newest peer set never arrived: the producer stalled or dropped it permanently")
		}
	}
}

// The production path builds its source from the Kubernetes client. It cannot
// be driven without an API server, but constructing it must not panic — that
// is the branch every real gateway takes.
func TestListerWatcher_UsesTheClientWhenNotOverridden(t *testing.T) {
	d := NewEndpointSliceDiscovery(k8sfake.NewSimpleClientset(),
		"openchoreo-control-plane", "cluster-gateway-mesh", 8445, "gw-self", testLogger())

	if d.newListerWatcher != nil {
		t.Fatal("the production constructor must not install a test seam")
	}
	if lw := d.listerWatcher(); lw == nil {
		t.Fatal("listerWatcher() returned nil for the production path")
	}
}
