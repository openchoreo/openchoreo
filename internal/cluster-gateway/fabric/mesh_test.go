// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fabric

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// staticDiscovery lets tests push peer sets by hand.
type staticDiscovery struct {
	ch chan []Peer
}

func newStaticDiscovery() *staticDiscovery {
	return &staticDiscovery{ch: make(chan []Peer, 8)}
}

func (d *staticDiscovery) Watch(ctx context.Context) (<-chan []Peer, error) {
	return d.ch, nil
}

// fakeDelegate records forwarded requests and plane events.
type fakeDelegate struct {
	mu       sync.Mutex
	forwards []*ForwardRequest
	events   []PlaneEvent
	noAgent  bool
	status   int
}

func (d *fakeDelegate) ServeForward(req *ForwardRequest) *ForwardResponse {
	d.mu.Lock()
	d.forwards = append(d.forwards, req)
	noAgent := d.noAgent
	status := d.status
	d.mu.Unlock()

	if noAgent {
		return &ForwardResponse{CorrID: req.CorrID, NoAgent: true, Error: "no agents found"}
	}
	if status == 0 {
		status = 200
	}
	return &ForwardResponse{
		CorrID:   req.CorrID,
		Response: &messaging.HTTPTunnelResponse{RequestID: req.Request.RequestID, StatusCode: status},
	}
}

func (d *fakeDelegate) ApplyPlaneEvent(ev PlaneEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, ev)
}

func (d *fakeDelegate) forwardCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.forwards)
}

func (d *fakeDelegate) eventCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.events)
}

type meshNode struct {
	mesh     *Mesh
	registry *Registry
	disc     *staticDiscovery
	delegate *fakeDelegate
	addr     string
}

func startMeshNode(t *testing.T, ctx context.Context, id string) *meshNode {
	t.Helper()

	registry := NewRegistry(testLogger())
	disc := newStaticDiscovery()
	delegate := &fakeDelegate{}
	mesh := NewMesh(MeshConfig{
		Self:       Peer{ID: id},
		ListenPort: 0, // ephemeral; no TLS in tests
	}, registry, disc, delegate, testLogger())

	if err := mesh.Start(ctx); err != nil {
		t.Fatalf("failed to start mesh %s: %v", id, err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mesh.Shutdown(shutdownCtx)
	})

	return &meshNode{mesh: mesh, registry: registry, disc: disc, delegate: delegate, addr: mesh.ListenAddr()}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// End-to-end over loopback: snapshot bootstrap, delta convergence, one-hop
// forwarding, plane event propagation, and drain — the full §03-§06 behavior
// of the fabric design.
func TestMesh_TwoNodeConvergenceAndForwarding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := startMeshNode(t, ctx, "gw-a")
	b := startMeshNode(t, ctx, "gw-b")

	// gw-a owns a connection before gw-b ever hears of it: the joining pod
	// must catch up via snapshot, not deltas.
	a.mesh.LocalUpsert(AgentEntry{PlaneIdentifier: "dataplane/dp-1", ConnID: "conn-1", ValidCRs: []string{"ns/cr-1"}})

	a.disc.ch <- []Peer{{ID: "gw-b", Addr: b.addr}}
	b.disc.ch <- []Peer{{ID: "gw-a", Addr: a.addr}}

	waitFor(t, "snapshot bootstrap on gw-b", func() bool {
		return len(b.registry.Lookup("dataplane/dp-1", "")) == 1
	})

	// Live delta convergence after bootstrap.
	a.mesh.LocalUpsert(AgentEntry{PlaneIdentifier: "dataplane/dp-1", ConnID: "conn-2"})
	waitFor(t, "delta add on gw-b", func() bool {
		return b.registry.CountForPlane("dataplane/dp-1") == 2
	})

	a.mesh.LocalRemove("dataplane/dp-1", "conn-2")
	waitFor(t, "delta remove on gw-b", func() bool {
		return b.registry.CountForPlane("dataplane/dp-1") == 1
	})

	// Worst case is one hop: gw-b forwards to gw-a, which answers as if it
	// had served the request itself.
	fwdCtx, fwdCancel := context.WithTimeout(ctx, 3*time.Second)
	defer fwdCancel()
	rsp, err := b.mesh.Forward(fwdCtx, "gw-a", &ForwardRequest{
		PlaneIdentifier: "dataplane/dp-1",
		CRKey:           "ns/cr-1",
		TimeoutMillis:   3000,
		Request:         &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET", Path: "/api/v1/pods"},
	})
	if err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	if rsp.Response == nil || rsp.Response.StatusCode != 200 {
		t.Fatalf("unexpected forward response: %+v", rsp)
	}

	// Plane events reach every replica, not just the one the notification
	// landed on.
	a.mesh.BroadcastPlaneEvent(PlaneEvent{PlaneType: "dataplane", PlaneID: "dp-1", Event: "updated"})
	waitFor(t, "plane event on gw-b", func() bool { return b.delegate.eventCount() == 1 })

	// DRAINING: peers stop routing new forwards to the draining pod while its
	// still-connected agents keep counting for status.
	a.mesh.Drain()
	waitFor(t, "draining visible on gw-b", func() bool {
		return len(b.registry.Lookup("dataplane/dp-1", "")) == 0
	})
	if got := b.registry.CountForPlane("dataplane/dp-1"); got != 1 {
		t.Fatalf("expected draining owner still counted, got %d", got)
	}
}

// A stale-registry forward is answered with NoAgent so the sender can retry a
// different candidate instead of failing the controller request.
func TestMesh_ForwardNoAgent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := startMeshNode(t, ctx, "gw-a")
	b := startMeshNode(t, ctx, "gw-b")
	a.delegate.noAgent = true

	a.disc.ch <- []Peer{{ID: "gw-b", Addr: b.addr}}
	b.disc.ch <- []Peer{{ID: "gw-a", Addr: a.addr}}

	waitFor(t, "link from gw-b to gw-a", func() bool {
		b.mesh.linksMu.Lock()
		link, ok := b.mesh.links["gw-a"]
		b.mesh.linksMu.Unlock()
		return ok && link.current() != nil
	})

	fwdCtx, fwdCancel := context.WithTimeout(ctx, 3*time.Second)
	defer fwdCancel()
	rsp, err := b.mesh.Forward(fwdCtx, "gw-a", &ForwardRequest{
		PlaneIdentifier: "dataplane/dp-1",
		Request:         &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET", Path: "/x"},
	})
	if err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	if !rsp.NoAgent {
		t.Fatalf("expected NoAgent response, got %+v", rsp)
	}
}

func TestMesh_ForwardWithoutLink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := startMeshNode(t, ctx, "gw-a")

	_, err := a.mesh.Forward(ctx, "gw-unknown", &ForwardRequest{
		Request: &messaging.HTTPTunnelRequest{Target: "k8s"},
	})
	if !errors.Is(err, ErrNoLink) {
		t.Fatalf("expected ErrNoLink, got %v", err)
	}
}

// When a pod vanishes from discovery, every peer drops its registry entries
// in one operation — ownership dies with the owner.
func TestMesh_PeerRemovalPurgesRegistry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	a := startMeshNode(t, ctx, "gw-a")
	b := startMeshNode(t, ctx, "gw-b")

	a.mesh.LocalUpsert(AgentEntry{PlaneIdentifier: "dataplane/dp-1", ConnID: "conn-1"})
	a.disc.ch <- []Peer{{ID: "gw-b", Addr: b.addr}}
	b.disc.ch <- []Peer{{ID: "gw-a", Addr: a.addr}}

	waitFor(t, "entry replicated to gw-b", func() bool {
		return b.registry.CountForPlane("dataplane/dp-1") == 1
	})

	b.disc.ch <- []Peer{}
	waitFor(t, "purge after peer removal", func() bool {
		return b.registry.CountForPlane("dataplane/dp-1") == 0
	})
}

// A forward waits on a reply that can only arrive over one peer's link. If
// that link drops the reply can never come, so the waiter must be released
// immediately rather than blocking until the request timeout — during a
// rolling restart the peer being forwarded to is routinely the pod going
// away, and the caller has other replicas it could retry instead.
func TestMesh_PendingForwardsReleasedOnLinkLoss(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		&staticDiscovery{ch: make(chan []Peer, 1)}, nil, testLogger())

	dying := make(chan *ForwardResponse, 1)
	survivor := make(chan *ForwardResponse, 1)
	m.pendingMu.Lock()
	m.pending["corr-dying"] = &pendingForward{ch: dying, owner: "gw-dying"}
	m.pending["corr-other"] = &pendingForward{ch: survivor, owner: "gw-live"}
	m.pendingMu.Unlock()

	m.failPendingForPeer("gw-dying")

	select {
	case got := <-dying:
		if got != nil {
			t.Fatalf("expected nil to signal a lost link, got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter on the lost link was not released")
	}

	m.pendingMu.Lock()
	_, cleared := m.pending["corr-dying"]
	_, kept := m.pending["corr-other"]
	m.pendingMu.Unlock()

	if cleared {
		t.Fatal("the lost peer's forward should have been removed")
	}
	if !kept {
		t.Fatal("forwards to other peers must be left alone")
	}
	if len(survivor) != 0 {
		t.Fatal("the live peer's waiter must not be signaled")
	}
}

// Readiness must not admit a pod whose registry is still empty: routing to it
// answers "no agents found" for planes whose agents are connected to peers.
func TestMesh_ConvergedGatesOnPeerSnapshots(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		&staticDiscovery{ch: make(chan []Peer, 1)}, nil, testLogger())

	m.convergeMu.Lock()
	m.startedAt = time.Now()
	m.convergeMu.Unlock()

	if m.Converged() {
		t.Fatal("must not be converged before discovery has reported")
	}

	// Discovery reports two peers; neither has sent a snapshot yet.
	m.convergeMu.Lock()
	m.peersSeen = true
	m.knownPeers = map[string]bool{"gw-a": true, "gw-b": true}
	m.convergeMu.Unlock()
	if m.Converged() {
		t.Fatal("must not be converged while peer snapshots are outstanding")
	}

	m.convergeMu.Lock()
	m.snapshotsFrom["gw-a"] = true
	m.convergeMu.Unlock()
	if m.Converged() {
		t.Fatal("must not be converged with only some peers reporting")
	}

	m.convergeMu.Lock()
	m.snapshotsFrom["gw-b"] = true
	m.convergeMu.Unlock()
	if !m.Converged() {
		t.Fatal("should be converged once every known peer has sent a snapshot")
	}
}

// A single-replica deployment has no peers, so it is converged as soon as
// discovery reports the empty set — it must not wait out the grace period.
func TestMesh_ConvergedWithNoPeers(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-solo"}}, NewRegistry(testLogger()),
		&staticDiscovery{ch: make(chan []Peer, 1)}, nil, testLogger())
	m.convergeMu.Lock()
	m.startedAt = time.Now()
	m.peersSeen = true
	m.convergeMu.Unlock()

	if !m.Converged() {
		t.Fatal("a peerless replica should be immediately converged")
	}
}

// An unreachable peer must not hold a replica out of service forever: past
// the grace cap the pod serves with whatever registry it has.
func TestMesh_ConvergedFailsOpenAfterGrace(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		&staticDiscovery{ch: make(chan []Peer, 1)}, nil, testLogger())
	m.convergeMu.Lock()
	m.startedAt = time.Now().Add(-meshConvergeGrace - time.Second)
	m.peersSeen = true
	m.knownPeers = map[string]bool{"gw-unreachable": true}
	m.convergeMu.Unlock()

	if !m.Converged() {
		t.Fatal("must fail open once the convergence grace period has elapsed")
	}
}

// A mesh certificate with no CA must be refused rather than downgraded to an
// unverified link: anything that can reach the mesh port would otherwise
// complete the handshake and inject registry deltas, which decide where agent
// traffic is routed. Running without TLS stays possible, but only by clearing
// CertFile outright.
func TestMesh_TLSWithoutCAFailsClosed(t *testing.T) {
	m := NewMesh(MeshConfig{
		Self:     Peer{ID: "gw-self"},
		CertFile: "/certs/tls.crt",
		KeyFile:  "/certs/tls.key",
	}, NewRegistry(testLogger()), newStaticDiscovery(), nil, testLogger())

	_, err := m.buildTLSConfigs()
	if err == nil {
		t.Fatal("a mesh certificate without a CA must be rejected")
	}
	if !strings.Contains(err.Error(), "mesh CA is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// No certificate at all is the explicit, loudly logged plaintext path used by
// tests and single-replica dev setups: it must keep working.
func TestMesh_NoCertDisablesTLS(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), nil, testLogger())

	serverTLS, err := m.buildTLSConfigs()
	if err != nil {
		t.Fatalf("plaintext mesh must be allowed: %v", err)
	}
	if serverTLS != nil {
		t.Fatal("no certificate configured must yield no server TLS config")
	}
}

// The receiving side closes any connection whose first frame is not HELLO, so
// an outbound link must not become reachable by the broadcaster before its
// handshake is on the wire. A delta that overtook HELLO would not merely
// arrive early - it would cost the entire connection, mid-rollout, exactly
// when local mutations and fresh dials coincide.
func TestMesh_HelloIsFirstFrameOnLink(t *testing.T) {
	firstFrame := make(chan meshFrame, 1)
	upgrader := websocket.Upgrader{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var f meshFrame
		if err := conn.ReadJSON(&f); err != nil {
			return
		}
		select {
		case firstFrame <- f:
		default:
		}
		// Hold the link open so the dialer does not redial mid-assertion.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer peer.Close()

	disc := newStaticDiscovery()
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}, ListenPort: 0},
		NewRegistry(testLogger()), disc, &fakeDelegate{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("failed to start mesh: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_ = m.Shutdown(shutdownCtx)
	})

	// Keep the broadcaster loaded with deltas so it is always contending for
	// the link the moment one is published.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			m.LocalUpsert(AgentEntry{
				PlaneIdentifier: "dataplane/prod",
				ConnID:          fmt.Sprintf("conn-%d", i),
				ValidCRs:        []string{"ns/dp1"},
			})
		}
	}()

	disc.ch <- []Peer{{ID: "gw-peer", Addr: strings.TrimPrefix(peer.URL, "http://")}}

	select {
	case f := <-firstFrame:
		if f.Type != frameHello {
			t.Fatalf("first frame on a fresh link was %q, want %q", f.Type, frameHello)
		}
		if f.PodID != "gw-self" {
			t.Fatalf("hello carried podID %q, want gw-self", f.PodID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("peer never received a frame")
	}
}

// Each outbound link starts a pinger. It must die with its link: the ping
// ticker is stopped on the way out, so a pinger that only watches ctx parks on
// channels that will never fire again and lives until the peer leaves
// discovery - accumulating one stranded goroutine per redial, each pinning the
// connection it can no longer use.
func TestMesh_ServeLinkRetiresPinger(t *testing.T) {
	// The peer asks for a snapshot and waits for the reply before reporting the
	// link established. That reply can only come from serveLink's read loop, so
	// it proves the pinger was started - otherwise this test would pass by
	// never reaching the code it is meant to cover.
	established := make(chan struct{}, 64)
	upgrader := websocket.Upgrader{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.WriteJSON(&meshFrame{Type: frameSnapshotReq}); err != nil {
			return
		}
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		established <- struct{}{}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer peer.Close()

	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), &fakeDelegate{}, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx = ctx

	// One warm-up session so lazily created runtime goroutines are not counted.
	runLinkSession(t, m, ctx, peer.URL, established)
	before := runtime.NumGoroutine()

	const sessions = 10
	for range sessions {
		runLinkSession(t, m, ctx, peer.URL, established)
	}

	// Retirement is asynchronous: poll rather than assume it has happened.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+2 {
		t.Fatalf("goroutines grew from %d to %d over %d link sessions: pingers are outliving their links",
			before, got, sessions)
	}
}

// runLinkSession dials the peer, serves the link as an outbound one until the
// peer confirms it is established, then drops the connection and waits for
// serveLink to return.
func runLinkSession(t *testing.T, m *Mesh, ctx context.Context, peerURL string, established <-chan struct{}) {
	t.Helper()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(peerURL, "http")+"/mesh", nil)
	if err != nil {
		t.Fatalf("failed to dial test peer: %v", err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	ws := &wsLink{conn: conn}
	served := make(chan struct{})
	go func() {
		m.serveLink(ctx, ws, "gw-peer", true)
		close(served)
	}()

	select {
	case <-established:
	case <-time.After(5 * time.Second):
		t.Fatal("link never reached serveLink's read loop")
	}

	conn.Close()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("serveLink did not return after the connection dropped")
	}
}

// --- unit coverage for the small surfaces the integration tests skip ---

func TestMesh_SelfIDAndListenAddr(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), nil, testLogger())

	if m.SelfID() != "gw-self" {
		t.Fatalf("SelfID() = %q, want gw-self", m.SelfID())
	}
	// Before Start there is no listener; callers must get an empty string
	// rather than a nil dereference.
	if addr := m.ListenAddr(); addr != "" {
		t.Fatalf("ListenAddr() before Start = %q, want empty", addr)
	}
}

// Shutdown must be safe on a mesh that never started: the gateway calls it on
// every shutdown path, including ones where Start failed early.
func TestMesh_ShutdownBeforeStart(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), nil, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown before Start = %v, want nil", err)
	}
}

// A full outbox drops the frame instead of blocking. Blocking here would stall
// whichever connection handler is registering an agent, so the mesh trades a
// delta for liveness and lets the peer repair via a sequence gap.
func TestMesh_EnqueueDropsWhenOutboxFull(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), nil, testLogger())

	// Fill the outbox without a broadcaster draining it.
	for i := range meshOutboxSize {
		m.LocalUpsert(AgentEntry{
			PlaneIdentifier: "dataplane/prod",
			ConnID:          fmt.Sprintf("conn-%d", i),
			ValidCRs:        []string{"ns/dp1"},
		})
	}
	if len(m.outbox) != meshOutboxSize {
		t.Fatalf("outbox holds %d frames, want %d", len(m.outbox), meshOutboxSize)
	}

	// One more must be dropped rather than block.
	done := make(chan struct{})
	go func() {
		m.LocalUpsert(AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "overflow"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("LocalUpsert blocked on a full outbox")
	}

	// The local entry is still recorded even though its delta was dropped, so
	// a snapshot repair will carry it.
	snap := m.localSnapshot()
	found := false
	for _, e := range snap.Entries {
		if e.ConnID == "overflow" {
			found = true
		}
	}
	if !found {
		t.Fatal("a dropped delta must still leave the entry in local state for snapshot repair")
	}
}

// LocalRemove for an unknown connection is a no-op: it must not bump the
// sequence, or peers would see a gap and repair for nothing.
func TestMesh_LocalRemoveUnknownConnIsNoop(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), nil, testLogger())

	m.LocalRemove("dataplane/prod", "never-registered")
	if got := m.localSnapshot().Seq; got != 0 {
		t.Fatalf("sequence advanced to %d on a no-op removal", got)
	}
	if len(m.outbox) != 0 {
		t.Fatalf("a no-op removal enqueued %d frames", len(m.outbox))
	}
}

// --- handleFrame: frames that must be rejected or ignored ---

func TestMesh_HandleFrameRejectsAndIgnores(t *testing.T) {
	tests := []struct {
		name  string
		seed  bool // register a routable gw-peer entry first
		frame *meshFrame
		check func(t *testing.T, m *Mesh, d *fakeDelegate)
	}{
		{
			name:  "delta whose owner does not match the link is dropped",
			frame: &meshFrame{Type: frameDelta, Delta: &Delta{Op: DeltaOpAdd, Owner: "gw-impostor", Seq: 1, Entry: AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c1"}}},
			check: func(t *testing.T, m *Mesh, _ *fakeDelegate) {
				if got := m.registry.Lookup("dataplane/prod", ""); len(got) != 0 {
					t.Fatalf("a forged delta was applied: %v", got)
				}
			},
		},
		{
			name:  "delta with no payload is ignored",
			frame: &meshFrame{Type: frameDelta},
			check: func(t *testing.T, m *Mesh, _ *fakeDelegate) {
				if got := m.registry.Lookup("dataplane/prod", ""); len(got) != 0 {
					t.Fatalf("an empty delta changed the registry: %v", got)
				}
			},
		},
		{
			name:  "snapshot whose owner does not match the link is dropped",
			frame: &meshFrame{Type: frameSnapshotRsp, Snapshot: &Snapshot{Owner: "gw-impostor", Seq: 1, Entries: []AgentEntry{{PlaneIdentifier: "dataplane/prod", ConnID: "c1"}}}},
			check: func(t *testing.T, m *Mesh, _ *fakeDelegate) {
				if got := m.registry.Lookup("dataplane/prod", ""); len(got) != 0 {
					t.Fatalf("a forged snapshot was applied: %v", got)
				}
			},
		},
		{
			name:  "draining frame for another pod is dropped",
			seed:  true,
			frame: &meshFrame{Type: frameDraining, PodID: "gw-someone-else"},
			check: func(t *testing.T, m *Mesh, _ *fakeDelegate) {
				// gw-peer's entry must remain routable: the notice named a
				// different pod, so it says nothing about this link's owner.
				if got := m.registry.Lookup("dataplane/prod", ""); len(got) != 1 {
					t.Fatalf("a draining frame naming another pod took gw-peer out of rotation: %v", got)
				}
			},
		},
		{
			name:  "forward response for an unknown correlation ID is ignored",
			frame: &meshFrame{Type: frameForwardRsp, ForwardRsp: &ForwardResponse{CorrID: "not-pending"}},
			check: func(t *testing.T, _ *Mesh, _ *fakeDelegate) {},
		},
		{
			name:  "forward request with no payload is ignored",
			frame: &meshFrame{Type: frameForwardReq},
			check: func(t *testing.T, _ *Mesh, d *fakeDelegate) {
				if d.forwardCount() != 0 {
					t.Fatal("an empty forward request reached the delegate")
				}
			},
		},
		{
			name:  "plane event with no payload is ignored",
			frame: &meshFrame{Type: framePlaneEvent},
			check: func(t *testing.T, _ *Mesh, d *fakeDelegate) {
				if d.eventCount() != 0 {
					t.Fatal("an empty plane event reached the delegate")
				}
			},
		},
		{
			name:  "hello on an established link is ignored",
			frame: &meshFrame{Type: frameHello, PodID: "gw-peer"},
			check: func(t *testing.T, _ *Mesh, _ *fakeDelegate) {},
		},
		{
			name:  "unknown frame types are ignored",
			frame: &meshFrame{Type: "not-a-frame-type"},
			check: func(t *testing.T, _ *Mesh, _ *fakeDelegate) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delegate := &fakeDelegate{}
			m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
				newStaticDiscovery(), delegate, testLogger())

			if tt.seed {
				m.registry.ApplyDelta(Delta{
					Op: DeltaOpAdd, Owner: "gw-peer", Seq: 1,
					Entry: AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c1"},
				})
			}

			// None of these frames reach a send, so a link with no socket is
			// enough to exercise the dispatch.
			m.handleFrame(&wsLink{}, "gw-peer", tt.frame)
			time.Sleep(20 * time.Millisecond) // delegate calls are dispatched async
			tt.check(t, m, delegate)
		})
	}
}

// A plane event from a peer must reach the delegate: it is how a pod learns
// that a plane it holds connections for was deleted elsewhere.
func TestMesh_HandleFrameDeliversPlaneEvent(t *testing.T) {
	delegate := &fakeDelegate{}
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), delegate, testLogger())

	m.handleFrame(&wsLink{}, "gw-peer", &meshFrame{
		Type:       framePlaneEvent,
		PlaneEvent: &PlaneEvent{PlaneType: "dataplane", PlaneID: "prod", Event: "deleted"},
	})

	waitFor(t, "the plane event to reach the delegate", func() bool {
		return delegate.eventCount() == 1
	})
}

// A draining frame from the pod that owns the link marks that owner draining,
// which is what stops new work being routed to a replica on its way out.
func TestMesh_HandleFrameAcceptsOwnDrainingNotice(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), nil, testLogger())

	m.registry.ApplyDelta(Delta{
		Op: DeltaOpAdd, Owner: "gw-peer", Seq: 1,
		Entry: AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c1"},
	})
	if got := m.registry.Lookup("dataplane/prod", ""); len(got) != 1 {
		t.Fatalf("precondition: gw-peer should be routable, got %v", got)
	}

	m.handleFrame(&wsLink{}, "gw-peer", &meshFrame{Type: frameDraining, PodID: "gw-peer"})

	if got := m.registry.Lookup("dataplane/prod", ""); len(got) != 0 {
		t.Fatalf("a draining peer must leave the rotation, still got %v", got)
	}
}

// writeMeshCertFiles writes a self-signed CA plus a leaf signed by it, the
// shape cert-manager produces for the gateway serving secret.
func writeMeshCertFiles(t *testing.T) (certPath, keyPath, caPath string) {
	t.Helper()
	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "mesh-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("failed to parse CA: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "cluster-gateway-mesh"},
		DNSNames:     []string{"cluster-gateway-mesh.openchoreo-control-plane.svc"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create leaf: %v", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("failed to marshal leaf key: %v", err)
	}

	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")
	caPath = filepath.Join(dir, "ca.crt")
	write := func(path string, block *pem.Block) {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}
	write(certPath, &pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	write(keyPath, &pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})
	write(caPath, &pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return certPath, keyPath, caPath
}

// With a certificate and CA in place the mesh must demand verified peers in
// both directions. Anything weaker would let a process that merely reaches the
// port join and inject registry deltas.
func TestMesh_BuildTLSConfigsRequiresMutualVerification(t *testing.T) {
	certPath, keyPath, caPath := writeMeshCertFiles(t)

	m := NewMesh(MeshConfig{
		Self:       Peer{ID: "gw-self"},
		CertFile:   certPath,
		KeyFile:    keyPath,
		CAFile:     caPath,
		ServerName: "cluster-gateway-mesh.openchoreo-control-plane.svc",
	}, NewRegistry(testLogger()), newStaticDiscovery(), nil, testLogger())

	serverTLS, err := m.buildTLSConfigs()
	if err != nil {
		t.Fatalf("buildTLSConfigs failed: %v", err)
	}

	if serverTLS.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("ClientAuth = %v, want RequireAndVerifyClientCert", serverTLS.ClientAuth)
	}
	if serverTLS.ClientCAs == nil {
		t.Fatal("server side must verify peers against the mesh CA")
	}
	if serverTLS.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", serverTLS.MinVersion)
	}
	if len(serverTLS.Certificates) != 1 {
		t.Fatal("server side must present the mesh certificate")
	}

	if m.clientTLS == nil {
		t.Fatal("client side config was not built")
	}
	if m.clientTLS.RootCAs == nil {
		t.Fatal("client side must verify peers against the mesh CA")
	}
	if m.clientTLS.InsecureSkipVerify {
		t.Fatal("client side must not skip verification")
	}
	if m.clientTLS.ServerName != "cluster-gateway-mesh.openchoreo-control-plane.svc" {
		t.Fatalf("ServerName = %q, want the mesh Service name", m.clientTLS.ServerName)
	}
}

// Every unusable TLS input must stop the mesh at startup rather than degrade
// it: a mesh that comes up without verification is worse than one that refuses
// to come up, because the failure is silent.
func TestMesh_BuildTLSConfigsRejectsBadInputs(t *testing.T) {
	certPath, keyPath, caPath := writeMeshCertFiles(t)
	dir := t.TempDir()

	notPEM := filepath.Join(dir, "not-a-cert.pem")
	if err := os.WriteFile(notPEM, []byte("this is not a certificate"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	tests := []struct {
		name    string
		cfg     MeshConfig
		wantErr string
	}{
		{
			name:    "certificate without a CA",
			cfg:     MeshConfig{CertFile: certPath, KeyFile: keyPath},
			wantErr: "mesh CA is required",
		},
		{
			name:    "unreadable certificate",
			cfg:     MeshConfig{CertFile: filepath.Join(dir, "missing.crt"), KeyFile: keyPath, CAFile: caPath},
			wantErr: "failed to load mesh certificate",
		},
		{
			name:    "key that does not match the certificate",
			cfg:     MeshConfig{CertFile: certPath, KeyFile: notPEM, CAFile: caPath},
			wantErr: "failed to load mesh certificate",
		},
		{
			name:    "unreadable CA",
			cfg:     MeshConfig{CertFile: certPath, KeyFile: keyPath, CAFile: filepath.Join(dir, "missing-ca.crt")},
			wantErr: "failed to read mesh CA",
		},
		{
			name:    "CA file with no certificates in it",
			cfg:     MeshConfig{CertFile: certPath, KeyFile: keyPath, CAFile: notPEM},
			wantErr: "failed to parse mesh CA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.Self = Peer{ID: "gw-self"}
			m := NewMesh(cfg, NewRegistry(testLogger()), newStaticDiscovery(), nil, testLogger())

			serverTLS, err := m.buildTLSConfigs()
			if err == nil {
				t.Fatal("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
			if serverTLS != nil {
				t.Fatal("no server config may be returned alongside an error")
			}
		})
	}
}

// The listener must refuse any connection that does not identify itself first.
// Identity is what every later frame is checked against — a link accepted
// without HELLO could claim to own any pod's registry entries.
func TestMesh_HandleMeshConnRejectsBadHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	node := startMeshNode(t, ctx, "gw-a")

	tests := []struct {
		name  string
		first *meshFrame
	}{
		{
			name:  "a delta before hello",
			first: &meshFrame{Type: frameDelta, Delta: &Delta{Op: DeltaOpAdd, Owner: "gw-impostor", Seq: 1}},
		},
		{
			name:  "hello without a pod ID",
			first: &meshFrame{Type: frameHello},
		},
		{
			name:  "an unknown frame type",
			first: &meshFrame{Type: "garbage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, resp, err := websocket.DefaultDialer.Dial("ws://"+node.addr+"/mesh", nil)
			if err != nil {
				t.Fatalf("dial failed: %v", err)
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			defer conn.Close()

			if err := conn.WriteJSON(tt.first); err != nil {
				t.Fatalf("failed to write first frame: %v", err)
			}

			// The server closes the socket; the read must end rather than hang.
			if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatalf("failed to set deadline: %v", err)
			}
			if _, _, err := conn.ReadMessage(); err == nil {
				t.Fatal("connection was accepted without a valid hello")
			}
		})
	}
}

// Forwarding to an owner we hold no link to must fail immediately with a
// retryable sentinel: the caller can try another replica, and the request
// provably never left this pod.
func TestMesh_ForwardErrorsAreClassified(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	node := startMeshNode(t, ctx, "gw-a")

	_, err := node.mesh.Forward(ctx, "gw-nowhere", &ForwardRequest{
		Request: &messaging.HTTPTunnelRequest{Target: "k8s", Method: "POST"},
	})
	if !errors.Is(err, ErrNoLink) {
		t.Fatalf("forward without a link = %v, want ErrNoLink", err)
	}
	if errors.Is(err, ErrForwardMayHaveExecuted) {
		t.Fatal("a request that never left must not be reported as possibly executed")
	}
}

// Purging or draining an owner the registry never heard of must be a no-op.
// Both are driven by peer lifecycle events, which can arrive for pods whose
// entries were already dropped.
func TestRegistry_UnknownOwnerOperationsAreNoops(t *testing.T) {
	r := NewRegistry(testLogger())
	r.ApplyDelta(Delta{
		Op: DeltaOpAdd, Owner: "gw-1", Seq: 1,
		Entry: AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c1"},
	})

	r.PurgeOwner("gw-never-seen")
	r.SetDraining("gw-never-seen")

	if got := r.Lookup("dataplane/prod", ""); len(got) != 1 {
		t.Fatalf("operations on an unknown owner disturbed the registry: %v", got)
	}
}

// A peer that cannot be dialed must be retried with a widening backoff rather
// than in a tight loop: during a rollout every replica briefly points at an
// address that is not accepting yet, and hammering it would turn a slow start
// into a thundering herd.
func TestMesh_RunLinkBacksOffOnDialFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bind and immediately release a port so the address is routable but
	// nothing is listening: dials fail fast and deterministically.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	deadAddr := ln.Addr().String()
	ln.Close()

	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), &fakeDelegate{}, testLogger())
	m.ctx = ctx

	link := &peerLink{peer: Peer{ID: "gw-dead", Addr: deadAddr}}
	linkCtx, linkCancel := context.WithCancel(ctx)

	done := make(chan struct{})
	go func() {
		m.runLink(linkCtx, link)
		close(done)
	}()

	// The first backoff is a second, so a link that were retrying without one
	// would burn many attempts in this window.
	time.Sleep(200 * time.Millisecond)
	if link.current() != nil {
		t.Fatal("a link that never dialed successfully must not be published")
	}

	// Canceling during the backoff wait must end the loop promptly rather
	// than serve out the remaining delay.
	linkCancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runLink did not exit while waiting out its backoff")
	}
}

// Forward must not block once the link it needs is gone: the caller is holding
// a request open, and the socket that would carry the answer no longer exists.
func TestMesh_ForwardFailsFastWithoutALink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	node := startMeshNode(t, ctx, "gw-a")

	start := time.Now()
	_, err := node.mesh.Forward(ctx, "gw-absent", &ForwardRequest{
		TimeoutMillis: 30_000,
		Request:       &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET"},
	})
	elapsed := time.Since(start)

	if !errors.Is(err, ErrNoLink) {
		t.Fatalf("Forward without a link = %v, want ErrNoLink", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Forward took %v: a missing link must fail immediately, not wait out the timeout", elapsed)
	}
}

// A dial that fails must be retried on a widening backoff, not immediately. A
// peer that is unreachable is usually unreachable for a while (pod restarting,
// endpoint stale), and a link that redialed on failure would spin a goroutine
// against it for as long as it stays in discovery.
func TestMesh_RunLinkWidensBackoffBetweenFailedDials(t *testing.T) {
	var mu sync.Mutex
	var attempts []time.Time

	// Answers the handshake with an error instead of upgrading, so every dial
	// fails at a real, fast, deterministic point rather than by timing out.
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts = append(attempts, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer peer.Close()

	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), &fakeDelegate{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx = ctx

	link := &peerLink{peer: Peer{ID: "gw-peer", Addr: strings.TrimPrefix(peer.URL, "http://")}}
	done := make(chan struct{})
	go func() {
		m.runLink(ctx, link)
		close(done)
	}()

	attemptsAt := func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		return append([]time.Time(nil), attempts...)
	}
	waitFor(t, "three dial attempts", func() bool { return len(attemptsAt()) >= 3 })

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runLink did not exit after its context was cancelled")
	}

	got := attemptsAt()
	first, second := got[1].Sub(got[0]), got[2].Sub(got[1])
	// The first backoff is a second; jitter only ever adds to it.
	if first < time.Second {
		t.Fatalf("redialed after %v, want at least 1s: a failed dial must not spin", first)
	}
	if second <= first {
		t.Fatalf("backoff did not widen: waited %v then %v", first, second)
	}
}

// failPendingForPeer releases forwards stranded by a dead link. One that was
// already answered has a full buffer, and must be stepped over rather than
// blocking the release of every forward queued behind it.
func TestMesh_FailPendingForPeerSkipsAlreadyAnsweredForwards(t *testing.T) {
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), &fakeDelegate{}, testLogger())

	// "answered" already holds its response, exactly as it would if the reply
	// landed in the instant before the link dropped.
	answered := make(chan *ForwardResponse, 1)
	answered <- &ForwardResponse{CorrID: "answered"}
	waiting := make(chan *ForwardResponse, 1)

	m.pendingMu.Lock()
	m.pending["answered"] = &pendingForward{ch: answered, owner: "gw-peer"}
	m.pending["waiting"] = &pendingForward{ch: waiting, owner: "gw-peer"}
	m.pending["other-peer"] = &pendingForward{ch: make(chan *ForwardResponse, 1), owner: "gw-elsewhere"}
	m.pendingMu.Unlock()

	release := make(chan struct{})
	go func() {
		m.failPendingForPeer("gw-peer")
		close(release)
	}()
	select {
	case <-release:
	case <-time.After(5 * time.Second):
		t.Fatal("failPendingForPeer blocked on a forward that was already answered")
	}

	select {
	case rsp := <-waiting:
		if rsp != nil {
			t.Fatalf("stranded forward got %+v, want nil to signal the link died", rsp)
		}
	default:
		t.Fatal("a forward stranded by the lost link was never released")
	}

	// The already-answered forward keeps its real response.
	select {
	case rsp := <-answered:
		if rsp == nil {
			t.Fatal("an answered forward had its response overwritten with nil")
		}
	default:
		t.Fatal("the answered forward lost its response")
	}

	// Forwards owned by a peer whose link is still up must be left alone.
	m.pendingMu.Lock()
	_, stillPending := m.pending["other-peer"]
	m.pendingMu.Unlock()
	if !stillPending {
		t.Fatal("a forward waiting on a healthy peer was failed too")
	}
}

// A sequence gap is repaired by asking the owner for a full snapshot. When that
// request cannot be written the link is already gone, so the failure is logged
// and the read loop left to notice — handleFrame must not panic or block.
func TestMesh_SnapshotRepairRequestFailureIsSurvivable(t *testing.T) {
	upgrader := websocket.Upgrader{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer peer.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(peer.URL, "http")+"/mesh", nil)
	if err != nil {
		t.Fatalf("failed to dial test peer: %v", err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	// Closing before the repair request is written is what makes the send fail.
	conn.Close()

	registry := NewRegistry(testLogger())
	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, registry,
		newStaticDiscovery(), &fakeDelegate{}, testLogger())

	// Seq 7 from an owner we have never heard from is a gap by definition.
	m.handleFrame(&wsLink{conn: conn}, "gw-peer", &meshFrame{
		Type: frameDelta,
		Delta: &Delta{
			Op: DeltaOpAdd, Owner: "gw-peer", Seq: 7,
			Entry: AgentEntry{PlaneIdentifier: "dataplane/prod", ConnID: "c1", ValidCRs: []string{"ns/dp1"}},
		},
	})

	// The gapped delta must not have been applied: routing to it would send
	// requests to a peer whose real state we do not know.
	if got := registry.Lookup("dataplane/prod", "ns/dp1"); len(got) != 0 {
		t.Fatalf("a delta that failed the sequence check was applied anyway: %+v", got)
	}
}

// Inbound links answer pings rather than sending them: the dialer drives
// keepalive. A peer that never gets a pong tears the link down at its pong
// deadline, so this reply is what keeps an accepted link alive.
func TestMesh_InboundLinkAnswersPeerPings(t *testing.T) {
	pongs := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetPongHandler(func(data string) error {
			select {
			case pongs <- data:
			default:
			}
			return nil
		})
		if err := conn.WriteControl(websocket.PingMessage, []byte("keepalive"),
			time.Now().Add(5*time.Second)); err != nil {
			return
		}
		// Control frames are only processed by a reader.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer peer.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(peer.URL, "http")+"/mesh", nil)
	if err != nil {
		t.Fatalf("failed to dial test peer: %v", err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	defer conn.Close()

	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), &fakeDelegate{}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	served := make(chan struct{})
	go func() {
		m.serveLink(ctx, &wsLink{conn: conn}, "gw-peer", false)
		close(served)
	}()

	select {
	case data := <-pongs:
		if data != "keepalive" {
			t.Fatalf("pong carried %q, want the ping payload %q", data, "keepalive")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("an inbound link never answered its peer's ping")
	}

	conn.Close()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("serveLink did not return after the connection dropped")
	}
}

// A caller that gives up must not leak the forward it was waiting on: pending
// entries are only ever removed by a response, a lost link, or this path, so a
// missed cleanup grows the map for the life of the pod.
func TestMesh_ForwardAbandonedWhenTheCallerGivesUp(t *testing.T) {
	upgrader := websocket.Upgrader{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Reads the forward and deliberately never answers it.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer peer.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(peer.URL, "http")+"/mesh", nil)
	if err != nil {
		t.Fatalf("failed to dial test peer: %v", err)
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	defer conn.Close()

	m := NewMesh(MeshConfig{Self: Peer{ID: "gw-self"}}, NewRegistry(testLogger()),
		newStaticDiscovery(), &fakeDelegate{}, testLogger())
	m.linksMu.Lock()
	m.links["gw-peer"] = &peerLink{peer: Peer{ID: "gw-peer"}, ws: &wsLink{conn: conn}}
	m.linksMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = m.Forward(ctx, "gw-peer", &ForwardRequest{
		PlaneIdentifier: "dataplane/prod",
		CRKey:           "ns/dp1",
		Request:         &messaging.HTTPTunnelRequest{Target: "k8s", Method: "GET"},
	})
	// The request was already on the wire, so the outcome is unknown - it must
	// not be reported as something the caller may safely retry.
	if !errors.Is(err, ErrForwardMayHaveExecuted) {
		t.Fatalf("abandoned forward returned %v, want ErrForwardMayHaveExecuted", err)
	}
	if errors.Is(err, ErrNoLink) || errors.Is(err, ErrForwardNotSent) {
		t.Fatalf("abandoned forward was reported as retry-safe: %v", err)
	}

	m.pendingMu.Lock()
	left := len(m.pending)
	m.pendingMu.Unlock()
	if left != 0 {
		t.Fatalf("abandoned forward left %d pending entries behind", left)
	}
}
