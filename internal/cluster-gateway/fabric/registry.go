// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fabric

import (
	"log/slog"
	"slices"
	"sort"
	"sync"
)

// Registry is the embedded RegistryStore: an in-memory map of agent
// connections owned by other gateway pods, kept converged by mesh deltas.
//
// Each owner pod is the sole source of truth for its own connections and
// stamps every delta with a per-owner monotonic sequence number. A gap means a
// missed message; the mesh repairs it by requesting a snapshot from the owner.
// Ownership dies with the owner: when a pod leaves the mesh its entries are
// dropped in one PurgeOwner call, so no tombstone bookkeeping is needed.
type Registry struct {
	mu     sync.RWMutex
	owners map[string]*ownerState
	// cursor implements per-pod round-robin over lookup results. Deliberately
	// unsynchronized across pods: the front-door LB already spreads requests
	// evenly, so per-pod cursors give approximately global fairness with zero
	// coordination.
	cursor map[string]int
	logger *slog.Logger
}

type ownerState struct {
	seq      uint64
	draining bool
	entries  map[string]AgentEntry // connID -> entry
}

// NewRegistry creates an empty registry.
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		owners: make(map[string]*ownerState),
		cursor: make(map[string]int),
		logger: logger.With("component", "fabric-registry"),
	}
}

// ApplyDelta applies a mutation from a peer. Returns false on a sequence gap
// (including an unknown owner whose numbering did not start at 1, e.g. after
// this pod missed earlier deltas or the owner restarted); the caller must
// repair by requesting a snapshot from the owner. Existing entries are kept
// while stale so routing can continue until the snapshot arrives.
func (r *Registry) ApplyDelta(d Delta) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, exists := r.owners[d.Owner]
	if !exists {
		if d.Seq != 1 {
			r.logger.Debug("delta for unknown owner requires snapshot",
				"owner", d.Owner,
				"seq", d.Seq,
			)
			return false
		}
		st = &ownerState{entries: make(map[string]AgentEntry)}
		r.owners[d.Owner] = st
	} else if d.Seq != st.seq+1 {
		r.logger.Warn("registry sequence gap detected",
			"owner", d.Owner,
			"expectedSeq", st.seq+1,
			"receivedSeq", d.Seq,
		)
		return false
	}

	st.seq = d.Seq
	switch d.Op {
	case DeltaOpAdd:
		st.entries[d.Entry.ConnID] = d.Entry
	case DeltaOpRemove:
		delete(st.entries, d.Entry.ConnID)
	default:
		r.logger.Warn("unknown delta op", "op", d.Op, "owner", d.Owner)
	}

	return true
}

// ApplySnapshot replaces all entries for the snapshot's owner, resetting the
// owner's sequence tracking. The draining flag is preserved so a DRAINING
// broadcast is not undone by a concurrent snapshot.
func (r *Registry) ApplySnapshot(s Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()

	draining := false
	if st, exists := r.owners[s.Owner]; exists {
		draining = st.draining
	}

	entries := make(map[string]AgentEntry, len(s.Entries))
	for _, e := range s.Entries {
		entries[e.ConnID] = e
	}
	r.owners[s.Owner] = &ownerState{
		seq:      s.Seq,
		draining: draining,
		entries:  entries,
	}

	r.logger.Info("applied registry snapshot",
		"owner", s.Owner,
		"seq", s.Seq,
		"entries", len(entries),
	)
}

// Lookup returns agent connections for the plane, optionally filtered to
// connections authorized for crKey, excluding draining owners. Results are
// rotated by a per-key cursor for round-robin fairness.
func (r *Registry) Lookup(planeIdentifier, crKey string) []RemoteAgent {
	r.mu.Lock()
	defer r.mu.Unlock()

	var results []RemoteAgent
	for owner, st := range r.owners {
		if st.draining {
			continue
		}
		for _, e := range st.entries {
			if e.PlaneIdentifier != planeIdentifier {
				continue
			}
			if crKey != "" && !slices.Contains(e.ValidCRs, crKey) {
				continue
			}
			results = append(results, RemoteAgent{Owner: owner, Entry: e})
		}
	}

	if len(results) == 0 {
		return nil
	}

	// Stable order, then rotate by the per-key cursor.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Owner != results[j].Owner {
			return results[i].Owner < results[j].Owner
		}
		return results[i].Entry.ConnID < results[j].Entry.ConnID
	})

	key := planeIdentifier + "/" + crKey
	offset := r.cursor[key] % len(results)
	r.cursor[key] = (offset + 1) % len(results)

	rotated := make([]RemoteAgent, 0, len(results))
	rotated = append(rotated, results[offset:]...)
	rotated = append(rotated, results[:offset]...)
	return rotated
}

// PurgeOwner drops all entries owned by a pod that left the mesh.
func (r *Registry) PurgeOwner(owner string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, exists := r.owners[owner]
	if !exists {
		return
	}
	delete(r.owners, owner)
	r.logger.Info("purged registry entries for departed owner",
		"owner", owner,
		"entriesDropped", len(st.entries),
	)
}

// SetDraining marks an owner as draining so its entries are excluded from
// Lookup while remaining visible for status reporting.
func (r *Registry) SetDraining(owner string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, exists := r.owners[owner]
	if !exists {
		st = &ownerState{entries: make(map[string]AgentEntry)}
		r.owners[owner] = st
	}
	st.draining = true
	r.logger.Info("owner marked draining", "owner", owner)
}

// CountForPlane returns the number of remote agent connections for a plane
// (draining owners included: their agents are still connected).
func (r *Registry) CountForPlane(planeIdentifier string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, st := range r.owners {
		for _, e := range st.entries {
			if e.PlaneIdentifier == planeIdentifier {
				count++
			}
		}
	}
	return count
}

// CountForCR returns the number of remote agent connections authorized for a
// specific CR within a plane.
func (r *Registry) CountForCR(planeIdentifier, crKey string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, st := range r.owners {
		for _, e := range st.entries {
			if e.PlaneIdentifier == planeIdentifier && slices.Contains(e.ValidCRs, crKey) {
				count++
			}
		}
	}
	return count
}

// AllPlaneCounts returns remote agent connection counts per plane identifier.
func (r *Registry) AllPlaneCounts() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int)
	for _, st := range r.owners {
		for _, e := range st.entries {
			counts[e.PlaneIdentifier]++
		}
	}
	return counts
}

// CountByOwner returns how many agent connections each peer pod currently
// owns, skipping pods that are draining (their connections are already on the
// way out, so counting them would skew a rebalance decision).
//
// Used to answer "am I holding more than my share?": connections concentrated
// on one replica turn that pod into a single point of failure, because
// restarting it evicts every agent at once.
func (r *Registry) CountByOwner() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int, len(r.owners))
	for owner, st := range r.owners {
		if st.draining {
			continue
		}
		counts[owner] = len(st.entries)
	}
	return counts
}
