// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package fabric implements the cluster gateway fabric: horizontal scaling of
// the websocket-terminating gateway by replicating the connection registry
// across replicas and forwarding requests between them (connect-to-one +
// gateway mesh).
//
// Each gateway pod keeps two kinds of state:
//   - local connections: the live agent websockets owned by this pod (held by
//     the gateway's ConnectionManager, never leave the pod)
//   - global registry: an in-memory, eventually-consistent view of every other
//     pod's connections, converged by broadcasting deltas over the mesh
//
// The three seams (PeerDiscovery, RegistryStore, Forwarder) are interfaces so
// the embedded defaults (EndpointSlice watch, in-memory registry, full-mesh
// links) can be swapped for external backends without touching request
// handling.
package fabric

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// Peer identifies a gateway replica in the mesh.
type Peer struct {
	// ID is the stable identity of the replica (pod name).
	ID string `json:"id"`
	// Addr is the host:port of the replica's mesh listener.
	Addr string `json:"addr"`
}

// AgentEntry describes one agent connection owned by a gateway pod.
type AgentEntry struct {
	PlaneIdentifier string   `json:"planeIdentifier"` // "{planeType}/{planeID}"
	ConnID          string   `json:"connID"`
	ValidCRs        []string `json:"validCRs,omitempty"`
}

// Delta is a single registry mutation, stamped with the owner pod's monotonic
// sequence number. Op "add" is an upsert (also used when a connection's
// ValidCRs change); "remove" deletes by ConnID.
type Delta struct {
	Op    string     `json:"op"` // "add" | "remove"
	Owner string     `json:"owner"`
	Seq   uint64     `json:"seq"`
	Entry AgentEntry `json:"entry"`
}

const (
	DeltaOpAdd    = "add"
	DeltaOpRemove = "remove"
)

// Snapshot is the full registry state for one owner pod, used to bootstrap a
// joining pod and to repair sequence gaps.
type Snapshot struct {
	Owner   string       `json:"owner"`
	Seq     uint64       `json:"seq"`
	Entries []AgentEntry `json:"entries"`
}

// PlaneEvent propagates plane lifecycle notifications (CR created / updated /
// deleted, manual reconnect) to every replica: the originating HTTP call lands
// on a single pod behind the load balancer, but agent connections for the
// plane may be owned by any pod.
type PlaneEvent struct {
	PlaneType string `json:"planeType"`
	PlaneID   string `json:"planeID"`
	Event     string `json:"event"` // "created" | "updated" | "deleted" | "reconnect"
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// ForwardRequest carries a controller request one hop over the mesh to the pod
// that owns an agent connection for the target plane.
type ForwardRequest struct {
	CorrID          string `json:"corrID"`
	PlaneIdentifier string `json:"planeIdentifier"`
	// CRKey selects a connection authorized for a specific CR
	// ("namespace/name"); empty means plane-level routing.
	CRKey         string                       `json:"crKey,omitempty"`
	TimeoutMillis int64                        `json:"timeoutMillis"`
	Request       *messaging.HTTPTunnelRequest `json:"request"`
}

// ForwardResponse is the reply to a ForwardRequest, correlated by CorrID.
type ForwardResponse struct {
	CorrID   string                        `json:"corrID"`
	Response *messaging.HTTPTunnelResponse `json:"response,omitempty"`
	// NoAgent reports that the owner pod had no (authorized) agent connection
	// for the request: the caller's registry view was stale and it should
	// retry against a different candidate.
	NoAgent bool   `json:"noAgent,omitempty"`
	Error   string `json:"error,omitempty"`
}

// RemoteAgent is a registry lookup result: an agent connection owned by
// another gateway pod.
type RemoteAgent struct {
	Owner string
	Entry AgentEntry
}

// PeerDiscovery watches the set of gateway replicas.
// The default implementation watches the EndpointSlices of the gateway's
// headless mesh Service.
type PeerDiscovery interface {
	// Watch delivers the full current peer set (excluding self) on every
	// change, starting with the initial state. The channel is closed when ctx
	// is cancelled.
	Watch(ctx context.Context) (<-chan []Peer, error)
}

// RegistryStore is the replicated view of agent connections owned by other
// pods. The hot path (Lookup) must be local-memory fast: no request ever
// blocks on a network call to find out where an agent lives.
type RegistryStore interface {
	// ApplyDelta applies a mutation from a peer. It returns false when a
	// sequence gap is detected for the owner: the caller must repair by
	// requesting a fresh Snapshot from that owner.
	ApplyDelta(d Delta) bool
	// ApplySnapshot replaces all entries for the snapshot's owner.
	ApplySnapshot(s Snapshot)
	// Lookup returns agent connections for the plane (optionally filtered to
	// connections authorized for crKey), excluding draining owners. Results
	// rotate on successive calls for per-pod round-robin fairness.
	Lookup(planeIdentifier, crKey string) []RemoteAgent
	// PurgeOwner drops all entries owned by a pod that left the mesh.
	PurgeOwner(owner string)
	// SetDraining marks an owner as draining: its entries stay visible for
	// status reporting but are excluded from Lookup.
	SetDraining(owner string)
}

// Forwarder sends a request one hop to the owner pod. The default
// implementation rides the already-open mesh link.
type Forwarder interface {
	Forward(ctx context.Context, owner string, req *ForwardRequest) (*ForwardResponse, error)
}

// Delegate is implemented by the gateway server: the mesh calls it to serve
// forwarded requests against locally-owned agent connections and to apply
// propagated plane lifecycle events.
type Delegate interface {
	// ServeForward handles a forwarded request using a locally-owned agent
	// connection and returns the response (or NoAgent when the registry view
	// that routed here was stale).
	ServeForward(req *ForwardRequest) *ForwardResponse
	// ApplyPlaneEvent applies a plane lifecycle event locally (revalidate or
	// disconnect agent connections owned by this pod).
	ApplyPlaneEvent(ev PlaneEvent)
}
