// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fabric

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Mesh frame types. One physical link between a pod pair carries any number
// of concurrent logical exchanges, multiplexed by CorrID (forwards) and
// per-owner sequence numbers (registry deltas).
const (
	frameHello       = "hello"
	frameDelta       = "delta"
	frameSnapshotReq = "snapshot_req"
	frameSnapshotRsp = "snapshot_rsp"
	frameForwardReq  = "forward_req"
	frameForwardRsp  = "forward_rsp"
	frameDraining    = "draining"
	framePlaneEvent  = "plane_event"
)

const (
	meshDialTimeout    = 10 * time.Second
	meshWriteTimeout   = 10 * time.Second
	meshPingInterval   = 30 * time.Second
	meshPongTimeout    = 90 * time.Second
	meshMaxDialBackoff = 15 * time.Second
	meshOutboxSize     = 1024
)

// ErrNoLink reports that no established mesh link exists for the owner pod.
// Returned before the request reaches the wire, so the caller may retry it on
// another candidate.
var ErrNoLink = errors.New("no mesh link to owner pod")

// ErrForwardNotSent reports that the forward frame could not be written to the
// link, so the owner never received it. Also safe to retry.
var ErrForwardNotSent = errors.New("mesh forward not sent")

// ErrForwardMayHaveExecuted reports that the request reached the wire but its
// outcome is unknown: the link dropped, or the deadline passed, with the
// forward in flight. The owner may already have dispatched it to an agent, so
// retrying risks applying a non-idempotent request twice.
var ErrForwardMayHaveExecuted = errors.New("mesh forward may have executed")

// meshFrame is the wire envelope for all gateway-to-gateway traffic.
type meshFrame struct {
	Type       string           `json:"type"`
	PodID      string           `json:"podID,omitempty"` // sender identity (hello, draining)
	Delta      *Delta           `json:"delta,omitempty"`
	Snapshot   *Snapshot        `json:"snapshot,omitempty"`
	ForwardReq *ForwardRequest  `json:"forwardReq,omitempty"`
	ForwardRsp *ForwardResponse `json:"forwardRsp,omitempty"`
	PlaneEvent *PlaneEvent      `json:"planeEvent,omitempty"`
}

// MeshConfig configures the embedded mesh transport.
type MeshConfig struct {
	// Self identifies this replica (ID = pod name).
	Self Peer
	// ListenPort is the mesh listener port.
	ListenPort int
	// CertFile/KeyFile is the TLS identity presented on both the server and
	// client side of mesh links. Empty CertFile disables TLS (tests/dev only).
	CertFile string
	KeyFile  string
	// CAFile verifies peer certificates in both directions. Required
	// whenever CertFile is set: an encrypted but unauthenticated mesh link
	// is rejected rather than silently accepted.
	CAFile string
	// ServerName is the name expected in peer certificates when dialing
	// (e.g. the headless mesh Service DNS name).
	ServerName string
}

// wsLink serializes writes to one websocket.
type wsLink struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (l *wsLink) send(f *meshFrame) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.conn.SetWriteDeadline(time.Now().Add(meshWriteTimeout)); err != nil {
		return err
	}
	return l.conn.WriteJSON(f)
}

// pendingForward is an in-flight forwarded request awaiting its response,
// tagged with the peer it was sent to so it can be released if that peer's
// link drops before answering.
type pendingForward struct {
	ch    chan *ForwardResponse
	owner string
}

// peerLink is an outbound link to one peer, redialed with backoff until the
// peer leaves the mesh.
type peerLink struct {
	peer   Peer
	cancel context.CancelFunc
	mu     sync.Mutex
	ws     *wsLink // nil while disconnected
}

func (pl *peerLink) current() *wsLink {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	return pl.ws
}

// Mesh is the embedded transport: a full mesh of long-lived websocket links
// between gateway replicas carrying registry deltas and forwarded requests.
// It implements Forwarder. Links are cheap at tens of replicas; at hundreds,
// swap this seam for a brokered transport.
type Mesh struct {
	cfg       MeshConfig
	registry  *Registry
	discovery PeerDiscovery
	delegate  Delegate
	logger    *slog.Logger

	// Local connection state owned by this pod (mirrored from the connection
	// manager). This pod is the sole source of truth for these entries; every
	// mutation bumps localSeq and is broadcast as a delta.
	localMu      sync.Mutex
	localSeq     uint64
	localEntries map[string]AgentEntry // connID -> entry

	linksMu sync.Mutex
	links   map[string]*peerLink

	pendingMu sync.Mutex
	pending   map[string]*pendingForward

	// Convergence tracking for readiness. A freshly started pod has an empty
	// registry: it does not yet know which peer owns which agent connection.
	// Serving traffic in that window produces "no agents found" even though
	// agents are connected elsewhere, so readiness waits until every peer
	// discovered so far has sent us its registry snapshot.
	convergeMu    sync.Mutex
	peersSeen     bool            // discovery has reported at least once
	knownPeers    map[string]bool // current peer set (excluding self)
	snapshotsFrom map[string]bool // peers whose snapshot we have applied
	startedAt     time.Time

	// outbox preserves broadcast ordering (enqueue order == seq order) without
	// doing network writes under localMu.
	outbox chan *meshFrame

	ctx        context.Context
	httpServer *http.Server
	listener   net.Listener
	upgrader   websocket.Upgrader
	clientTLS  *tls.Config
}

// NewMesh creates the mesh. Call Start to open the listener and begin peer
// discovery.
func NewMesh(cfg MeshConfig, registry *Registry, discovery PeerDiscovery, delegate Delegate, logger *slog.Logger) *Mesh {
	return &Mesh{
		cfg:           cfg,
		registry:      registry,
		discovery:     discovery,
		delegate:      delegate,
		logger:        logger.With("component", "fabric-mesh", "self", cfg.Self.ID),
		localEntries:  make(map[string]AgentEntry),
		links:         make(map[string]*peerLink),
		pending:       make(map[string]*pendingForward),
		outbox:        make(chan *meshFrame, meshOutboxSize),
		knownPeers:    make(map[string]bool),
		snapshotsFrom: make(map[string]bool),
	}
}

// SelfID returns this replica's mesh identity (pod name).
func (m *Mesh) SelfID() string {
	return m.cfg.Self.ID
}

// meshConvergeGrace caps how long readiness waits for peer snapshots. Past it
// the pod reports ready regardless: an unreachable peer must not be able to
// hold a replica out of service indefinitely, and serving with a partial
// registry still beats serving nothing. Forwards to a peer we have not heard
// from fail fast and retry elsewhere.
const meshConvergeGrace = 20 * time.Second

// Converged reports whether this pod's registry is populated enough to route.
//
// A pod that has just started knows nothing about which peer owns which agent
// connection, so requests arriving before its first snapshots come back are
// answered with "no agents found" even though agents are connected to peers.
// Readiness therefore waits for a snapshot from every peer discovery has told
// us about — bounded by meshConvergeGrace so a missing peer cannot wedge the
// pod permanently unready.
func (m *Mesh) Converged() bool {
	m.convergeMu.Lock()
	defer m.convergeMu.Unlock()

	// Discovery has not reported yet: we do not even know whether we have
	// peers, so we cannot claim a usable view.
	if !m.peersSeen {
		return !m.startedAt.IsZero() && time.Since(m.startedAt) > meshConvergeGrace
	}

	for peerID := range m.knownPeers {
		if !m.snapshotsFrom[peerID] {
			return time.Since(m.startedAt) > meshConvergeGrace
		}
	}
	return true
}

// Start opens the mesh listener, starts the broadcaster, and begins watching
// peer discovery. It returns after setup; runtime errors are logged and links
// self-heal via redial + snapshot repair.
func (m *Mesh) Start(ctx context.Context) error {
	m.ctx = ctx

	m.convergeMu.Lock()
	m.startedAt = time.Now()
	m.convergeMu.Unlock()

	serverTLS, err := m.buildTLSConfigs()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/mesh", m.handleMeshConn)

	m.httpServer = &http.Server{
		Handler:           mux,
		TLSConfig:         serverTLS,
		ReadHeaderTimeout: meshDialTimeout,
	}

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(m.cfg.ListenPort))
	if err != nil {
		return fmt.Errorf("failed to listen on mesh port %d: %w", m.cfg.ListenPort, err)
	}
	m.listener = listener

	go func() {
		var serveErr error
		if serverTLS != nil {
			serveErr = m.httpServer.ServeTLS(listener, "", "")
		} else {
			serveErr = m.httpServer.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			m.logger.Error("mesh server error", "error", serveErr)
		}
	}()

	go m.runBroadcaster(ctx)

	peerCh, err := m.discovery.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to start peer discovery: %w", err)
	}
	go func() {
		for peers := range peerCh {
			m.reconcilePeers(ctx, peers)
		}
	}()

	m.logger.Info("mesh started",
		"port", m.cfg.ListenPort,
		"tls", m.cfg.CertFile != "",
	)
	return nil
}

// Shutdown closes the mesh listener. Outbound links stop when the Start
// context is cancelled.
func (m *Mesh) Shutdown(ctx context.Context) error {
	if m.httpServer == nil {
		return nil
	}
	return m.httpServer.Shutdown(ctx)
}

// ListenAddr returns the mesh listener address after Start (useful when
// ListenPort is 0, e.g. in tests).
func (m *Mesh) ListenAddr() string {
	if m.listener == nil {
		return ""
	}
	return m.listener.Addr().String()
}

func (m *Mesh) buildTLSConfigs() (*tls.Config, error) {
	if m.cfg.CertFile == "" {
		m.logger.Warn("mesh TLS disabled: no certificate configured",
			"note", "mesh links are unencrypted; configure mesh certificates for production",
		)
		return nil, nil
	}

	// Fail closed the same way the cluster agent does when TLS is on with no
	// CA. Encryption without peer verification is the worse of the two
	// states: anything that can reach the mesh port could complete the
	// handshake, join as a peer and inject registry deltas, and those deltas
	// decide where agent traffic is routed. Disabling mesh TLS outright
	// (empty CertFile) stays the only way to run without verification, and it
	// is an explicit, loudly logged choice.
	if m.cfg.CAFile == "" {
		return nil, errors.New("mesh CA is required when a mesh certificate is configured: " +
			"set CAFile to the bundle that verifies mesh peers, or clear CertFile to disable mesh TLS")
	}

	cert, err := tls.LoadX509KeyPair(m.cfg.CertFile, m.cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load mesh certificate: %w", err)
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	m.clientTLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		ServerName:   m.cfg.ServerName,
	}

	caData, err := os.ReadFile(m.cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read mesh CA %s: %w", m.cfg.CAFile, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("failed to parse mesh CA %s: no valid certificates found", m.cfg.CAFile)
	}
	serverTLS.ClientAuth = tls.RequireAndVerifyClientCert
	serverTLS.ClientCAs = caPool
	m.clientTLS.RootCAs = caPool

	return serverTLS, nil
}

// --- local registry state (this pod as owner) ---

// LocalUpsert records an agent connection owned by this pod (new connection
// or changed CR authorizations) and broadcasts the delta.
func (m *Mesh) LocalUpsert(e AgentEntry) {
	m.localMu.Lock()
	m.localSeq++
	m.localEntries[e.ConnID] = e
	d := &Delta{Op: DeltaOpAdd, Owner: m.cfg.Self.ID, Seq: m.localSeq, Entry: e}
	m.enqueueLocked(&meshFrame{Type: frameDelta, Delta: d})
	m.localMu.Unlock()
}

// LocalRemove records the removal of an agent connection owned by this pod
// and broadcasts the delta.
func (m *Mesh) LocalRemove(planeIdentifier, connID string) {
	m.localMu.Lock()
	if _, ok := m.localEntries[connID]; !ok {
		m.localMu.Unlock()
		return
	}
	m.localSeq++
	delete(m.localEntries, connID)
	d := &Delta{
		Op:    DeltaOpRemove,
		Owner: m.cfg.Self.ID,
		Seq:   m.localSeq,
		Entry: AgentEntry{PlaneIdentifier: planeIdentifier, ConnID: connID},
	}
	m.enqueueLocked(&meshFrame{Type: frameDelta, Delta: d})
	m.localMu.Unlock()
}

func (m *Mesh) localSnapshot() Snapshot {
	m.localMu.Lock()
	defer m.localMu.Unlock()

	entries := make([]AgentEntry, 0, len(m.localEntries))
	for _, e := range m.localEntries {
		entries = append(entries, e)
	}
	return Snapshot{Owner: m.cfg.Self.ID, Seq: m.localSeq, Entries: entries}
}

// enqueueLocked queues a frame for broadcast. Called with localMu held for
// deltas so enqueue order matches sequence order; a full outbox drops the
// frame (peers repair via gap-triggered snapshots).
func (m *Mesh) enqueueLocked(f *meshFrame) {
	select {
	case m.outbox <- f:
	default:
		m.logger.Warn("mesh outbox full, dropping frame",
			"type", f.Type,
			"note", "peers will repair via snapshot on the next sequence gap",
		)
	}
}

// Drain broadcasts DRAINING so peers stop routing new forwards to this pod.
// Existing links stay up to carry in-flight responses and removal deltas.
func (m *Mesh) Drain() {
	m.localMu.Lock()
	m.enqueueLocked(&meshFrame{Type: frameDraining, PodID: m.cfg.Self.ID})
	m.localMu.Unlock()
	m.logger.Info("broadcast DRAINING to mesh peers")
}

// BroadcastPlaneEvent propagates a plane lifecycle event to all peers.
func (m *Mesh) BroadcastPlaneEvent(ev PlaneEvent) {
	m.localMu.Lock()
	m.enqueueLocked(&meshFrame{Type: framePlaneEvent, PlaneEvent: &ev})
	m.localMu.Unlock()
}

func (m *Mesh) runBroadcaster(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-m.outbox:
			for _, link := range m.snapshotLinks() {
				ws := link.current()
				if ws == nil {
					continue
				}
				if err := ws.send(f); err != nil {
					m.logger.Debug("mesh broadcast failed",
						"peer", link.peer.ID,
						"type", f.Type,
						"error", err,
					)
				}
			}
		}
	}
}

func (m *Mesh) snapshotLinks() []*peerLink {
	m.linksMu.Lock()
	defer m.linksMu.Unlock()
	links := make([]*peerLink, 0, len(m.links))
	for _, l := range m.links {
		links = append(links, l)
	}
	return links
}

// --- peer link management ---

func (m *Mesh) reconcilePeers(ctx context.Context, peers []Peer) {
	want := make(map[string]Peer, len(peers))
	for _, p := range peers {
		want[p.ID] = p
	}

	m.convergeMu.Lock()
	m.peersSeen = true
	m.knownPeers = make(map[string]bool, len(want))
	for id := range want {
		m.knownPeers[id] = true
	}
	m.convergeMu.Unlock()

	m.linksMu.Lock()
	var purged []string
	for id, link := range m.links {
		p, stillWanted := want[id]
		if stillWanted && p.Addr == link.peer.Addr {
			continue
		}
		link.cancel()
		delete(m.links, id)
		if !stillWanted {
			purged = append(purged, id)
		}
	}
	for id, p := range want {
		if _, exists := m.links[id]; exists {
			continue
		}
		lctx, cancel := context.WithCancel(ctx)
		link := &peerLink{peer: p, cancel: cancel}
		m.links[id] = link
		go m.runLink(lctx, link)
	}
	m.linksMu.Unlock()

	// Ownership dies with the owner: a pod gone from EndpointSlices loses all
	// its registry entries in one operation.
	for _, id := range purged {
		m.registry.PurgeOwner(id)
	}
}

func (m *Mesh) runLink(ctx context.Context, link *peerLink) {
	backoff := time.Second

	// waitBeforeRedial serves out the current backoff and widens it. Reports
	// false when the link is shutting down.
	waitBeforeRedial := func() bool {
		// Non-cryptographic randomness is fine for retry jitter.
		jitter := time.Duration(rand.Int64N(int64(backoff / 2))) // #nosec G404
		select {
		case <-ctx.Done():
			return false
		case <-time.After(backoff + jitter):
		}
		backoff = min(backoff*2, meshMaxDialBackoff)
		return true
	}

	for ctx.Err() == nil {
		ws, err := m.dial(ctx, link.peer)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.logger.Warn("mesh dial failed",
				"peer", link.peer.ID,
				"addr", link.peer.Addr,
				"retryIn", backoff,
				"error", err,
			)
			if !waitBeforeRedial() {
				return
			}
			continue
		}

		// Identify ourselves before the socket becomes reachable by anyone
		// else. The broadcaster and Forward write to whatever link.current()
		// returns, and the peer closes any connection whose first frame is not
		// HELLO — so a delta winning that race would cost the whole
		// connection, not just its own delivery.
		if err := ws.send(&meshFrame{Type: frameHello, PodID: m.cfg.Self.ID}); err != nil {
			m.logger.Warn("mesh hello failed",
				"peer", link.peer.ID,
				"retryIn", backoff,
				"error", err,
			)
			ws.conn.Close()
			// This socket never carried a frame, so treat it like a failed
			// dial: redialing immediately would spin against a peer that is
			// accepting connections but cannot serve them.
			if !waitBeforeRedial() {
				return
			}
			continue
		}

		link.mu.Lock()
		link.ws = ws
		link.mu.Unlock()
		backoff = time.Second

		m.logger.Info("mesh link established", "peer", link.peer.ID, "addr", link.peer.Addr)

		// Bootstrap: push our own current state, then pull the peer's full
		// registry state. The proactive push closes a race that gap-triggered
		// repair cannot: the broadcaster only sends deltas over links that are
		// already up (see runBroadcaster), so a local mutation that happens
		// before this dial completes is skipped for this peer and never
		// resent — there's no future delta to reveal a sequence gap. Pushing
		// our full state unconditionally on every fresh dial guarantees the
		// peer converges regardless of that timing. Subsequent deltas arrive
		// on this same outbound link.
		snapshot := m.localSnapshot()
		if err := ws.send(&meshFrame{Type: frameSnapshotRsp, Snapshot: &snapshot}); err != nil {
			m.logger.Warn("mesh self-snapshot push failed", "peer", link.peer.ID, "error", err)
		}
		if err := ws.send(&meshFrame{Type: frameSnapshotReq}); err != nil {
			m.logger.Warn("mesh snapshot request failed", "peer", link.peer.ID, "error", err)
		}
		m.serveLink(ctx, ws, link.peer.ID, true)

		link.mu.Lock()
		link.ws = nil
		link.mu.Unlock()
		ws.conn.Close()

		// Anything still awaiting a reply from this peer will never get one:
		// the socket that would have carried it is gone. Release those waiters
		// now so callers can retry another replica rather than blocking for the
		// full request timeout — the common case during a rolling restart,
		// where the peer we forwarded to is the pod being replaced.
		m.failPendingForPeer(link.peer.ID)

		if ctx.Err() == nil {
			m.logger.Info("mesh link lost, redialing", "peer", link.peer.ID)
		}
	}
}

// failPendingForPeer releases every forward still waiting on a peer whose link
// has dropped. A nil response tells Forward the link died mid-request.
func (m *Mesh) failPendingForPeer(peerID string) {
	m.pendingMu.Lock()
	orphaned := make([]*pendingForward, 0)
	for corrID, pf := range m.pending {
		if pf.owner == peerID {
			orphaned = append(orphaned, pf)
			delete(m.pending, corrID)
		}
	}
	m.pendingMu.Unlock()

	if len(orphaned) == 0 {
		return
	}

	m.logger.Info("failing in-flight mesh forwards for lost peer link",
		"peer", peerID,
		"forwards", len(orphaned),
	)

	for _, pf := range orphaned {
		select {
		case pf.ch <- nil:
		default:
		}
	}
}

func (m *Mesh) dial(ctx context.Context, peer Peer) (*wsLink, error) {
	scheme := "ws"
	if m.clientTLS != nil {
		scheme = "wss"
	}
	u := url.URL{Scheme: scheme, Host: peer.Addr, Path: "/mesh"}

	dialer := websocket.Dialer{
		HandshakeTimeout: meshDialTimeout,
		TLSClientConfig:  m.clientTLS,
	}
	dialCtx, cancel := context.WithTimeout(ctx, meshDialTimeout)
	defer cancel()

	conn, resp, err := dialer.DialContext(dialCtx, u.String(), nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return &wsLink{conn: conn}, nil
}

// handleMeshConn accepts an inbound link from a peer. The first frame must be
// HELLO carrying the peer's identity.
func (m *Mesh) handleMeshConn(w http.ResponseWriter, r *http.Request) {
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.logger.Warn("mesh upgrade failed", "remote", r.RemoteAddr, "error", err)
		return
	}
	ws := &wsLink{conn: conn}

	if err := conn.SetReadDeadline(time.Now().Add(meshDialTimeout)); err != nil {
		conn.Close()
		return
	}
	var hello meshFrame
	if err := conn.ReadJSON(&hello); err != nil || hello.Type != frameHello || hello.PodID == "" {
		m.logger.Warn("mesh connection rejected: missing hello", "remote", r.RemoteAddr)
		conn.Close()
		return
	}

	m.logger.Info("mesh peer connected", "peer", hello.PodID, "remote", r.RemoteAddr)
	m.serveLink(m.ctx, ws, hello.PodID, false)
	conn.Close()
}

// serveLink runs the shared frame dispatch loop for one link until the link
// fails or ctx is cancelled. Both link directions understand the full frame
// set, which keeps gap repair link-local: a gap detected on an inbound link is
// repaired by a snapshot exchange on that same socket.
func (m *Mesh) serveLink(ctx context.Context, ws *wsLink, peerID string, outbound bool) {
	stop := context.AfterFunc(ctx, func() { ws.conn.Close() })
	defer stop()

	if err := ws.conn.SetReadDeadline(time.Now().Add(meshPongTimeout)); err != nil {
		return
	}
	if outbound {
		ws.conn.SetPongHandler(func(string) error {
			return ws.conn.SetReadDeadline(time.Now().Add(meshPongTimeout))
		})
		pingTicker := time.NewTicker(meshPingInterval)
		defer pingTicker.Stop()
		// Retire the pinger when this link ends, not only when the peer's
		// context is cancelled. Stopping the ticker silences the very channel
		// the goroutine parks on, so without this signal it waits on two
		// channels that will never fire again - one goroutine stranded per
		// redial, each holding the dead connection, for as long as the peer
		// stays in discovery.
		linkDone := make(chan struct{})
		defer close(linkDone)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-linkDone:
					return
				case <-pingTicker.C:
					ws.mu.Lock()
					err := ws.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(meshWriteTimeout))
					ws.mu.Unlock()
					if err != nil {
						return
					}
				}
			}
		}()
	} else {
		ws.conn.SetPingHandler(func(appData string) error {
			if err := ws.conn.SetReadDeadline(time.Now().Add(meshPongTimeout)); err != nil {
				return err
			}
			ws.mu.Lock()
			defer ws.mu.Unlock()
			return ws.conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(meshWriteTimeout))
		})
	}

	for {
		var f meshFrame
		if err := ws.conn.ReadJSON(&f); err != nil {
			if ctx.Err() == nil {
				m.logger.Debug("mesh link read ended", "peer", peerID, "error", err)
			}
			return
		}
		if !outbound {
			if err := ws.conn.SetReadDeadline(time.Now().Add(meshPongTimeout)); err != nil {
				return
			}
		}
		m.handleFrame(ws, peerID, &f)
	}
}

func (m *Mesh) handleFrame(ws *wsLink, peerID string, f *meshFrame) {
	switch f.Type {
	case frameDelta:
		if f.Delta == nil || f.Delta.Owner != peerID {
			m.logger.Warn("ignoring delta with mismatched owner", "peer", peerID)
			return
		}
		if !m.registry.ApplyDelta(*f.Delta) {
			// Sequence gap: repair by pulling the peer's full state over this
			// same link.
			if err := ws.send(&meshFrame{Type: frameSnapshotReq}); err != nil {
				m.logger.Warn("mesh snapshot repair request failed", "peer", peerID, "error", err)
			}
		}

	case frameSnapshotReq:
		snapshot := m.localSnapshot()
		if err := ws.send(&meshFrame{Type: frameSnapshotRsp, Snapshot: &snapshot}); err != nil {
			m.logger.Warn("mesh snapshot response failed", "peer", peerID, "error", err)
		}

	case frameSnapshotRsp:
		if f.Snapshot == nil || f.Snapshot.Owner != peerID {
			m.logger.Warn("ignoring snapshot with mismatched owner", "peer", peerID)
			return
		}
		m.registry.ApplySnapshot(*f.Snapshot)
		m.convergeMu.Lock()
		m.snapshotsFrom[peerID] = true
		m.convergeMu.Unlock()

	case frameForwardReq:
		if f.ForwardReq == nil {
			return
		}
		req := f.ForwardReq
		go func() {
			rsp := m.delegate.ServeForward(req)
			rsp.CorrID = req.CorrID
			if err := ws.send(&meshFrame{Type: frameForwardRsp, ForwardRsp: rsp}); err != nil {
				m.logger.Warn("mesh forward response send failed",
					"peer", peerID,
					"corrID", req.CorrID,
					"error", err,
				)
			}
		}()

	case frameForwardRsp:
		if f.ForwardRsp == nil {
			return
		}
		m.pendingMu.Lock()
		pf, ok := m.pending[f.ForwardRsp.CorrID]
		if ok {
			delete(m.pending, f.ForwardRsp.CorrID)
		}
		m.pendingMu.Unlock()
		if !ok {
			m.logger.Debug("forward response for unknown corrID", "corrID", f.ForwardRsp.CorrID)
			return
		}
		pf.ch <- f.ForwardRsp

	case frameDraining:
		if f.PodID != peerID {
			m.logger.Warn("ignoring draining frame with mismatched pod", "peer", peerID)
			return
		}
		m.registry.SetDraining(peerID)

	case framePlaneEvent:
		if f.PlaneEvent == nil {
			return
		}
		ev := *f.PlaneEvent
		go m.delegate.ApplyPlaneEvent(ev)

	case frameHello:
		// Identity is established at link setup; ignore.

	default:
		m.logger.Warn("unknown mesh frame type", "type", f.Type, "peer", peerID)
	}
}

// --- Forwarder ---

// Forward sends a request one hop to the owner pod over the already-open mesh
// link and waits for the correlated response.
func (m *Mesh) Forward(ctx context.Context, owner string, req *ForwardRequest) (*ForwardResponse, error) {
	m.linksMu.Lock()
	link, exists := m.links[owner]
	m.linksMu.Unlock()
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrNoLink, owner)
	}
	ws := link.current()
	if ws == nil {
		return nil, fmt.Errorf("%w: %s (link down)", ErrNoLink, owner)
	}

	if req.CorrID == "" {
		req.CorrID = uuid.New().String()
	}

	ch := make(chan *ForwardResponse, 1)
	m.pendingMu.Lock()
	m.pending[req.CorrID] = &pendingForward{ch: ch, owner: owner}
	m.pendingMu.Unlock()

	cleanup := func() {
		m.pendingMu.Lock()
		delete(m.pending, req.CorrID)
		m.pendingMu.Unlock()
	}

	if err := ws.send(&meshFrame{Type: frameForwardReq, ForwardReq: req}); err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: to %s: %w", ErrForwardNotSent, owner, err)
	}

	select {
	case rsp := <-ch:
		// nil means the link to the owner dropped with this forward in flight
		// (see failPendingForPeer): report it now rather than waiting out the
		// timeout. Deliberately not ErrNoLink - the request was already on the
		// wire, so whether it ran is unknown and a blind retry could repeat it.
		if rsp == nil {
			return nil, fmt.Errorf("%w: %s (link lost mid-request)", ErrForwardMayHaveExecuted, owner)
		}
		return rsp, nil
	case <-ctx.Done():
		cleanup()
		return nil, fmt.Errorf("%w: to %s: %w", ErrForwardMayHaveExecuted, owner, ctx.Err())
	}
}
