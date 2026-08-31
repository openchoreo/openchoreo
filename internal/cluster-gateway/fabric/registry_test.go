// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fabric

import (
	"io"
	"log/slog"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func addDelta(owner string, seq uint64, planeIdentifier, connID string, validCRs ...string) Delta {
	return Delta{
		Op:    DeltaOpAdd,
		Owner: owner,
		Seq:   seq,
		Entry: AgentEntry{PlaneIdentifier: planeIdentifier, ConnID: connID, ValidCRs: validCRs},
	}
}

// The registry must apply in-order deltas so a request landing on any pod can
// find agents owned by peers — the core of connect-to-one + mesh routing.
func TestRegistry_ApplyDeltaAndLookup(t *testing.T) {
	r := NewRegistry(testLogger())

	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-1", "ns/cr-1")) {
		t.Fatal("expected first delta (seq 1) from unknown owner to apply")
	}
	if !r.ApplyDelta(addDelta("gw-1", 2, "dataplane/dp-b", "conn-2", "ns/cr-2")) {
		t.Fatal("expected in-order delta to apply")
	}

	agents := r.Lookup("dataplane/dp-a", "")
	if len(agents) != 1 || agents[0].Owner != "gw-1" || agents[0].Entry.ConnID != "conn-1" {
		t.Fatalf("unexpected lookup result: %+v", agents)
	}

	// CR filtering enforces per-CR security boundaries across the mesh.
	if got := r.Lookup("dataplane/dp-a", "ns/cr-1"); len(got) != 1 {
		t.Fatalf("expected authorized CR lookup to match, got %+v", got)
	}
	if got := r.Lookup("dataplane/dp-a", "ns/other"); len(got) != 0 {
		t.Fatalf("expected unauthorized CR lookup to be empty, got %+v", got)
	}

	// Remove deletes by connID.
	if !r.ApplyDelta(Delta{Op: DeltaOpRemove, Owner: "gw-1", Seq: 3,
		Entry: AgentEntry{PlaneIdentifier: "dataplane/dp-a", ConnID: "conn-1"}}) {
		t.Fatal("expected remove delta to apply")
	}
	if got := r.Lookup("dataplane/dp-a", ""); len(got) != 0 {
		t.Fatalf("expected entry removed, got %+v", got)
	}
}

// A sequence gap means a missed delta: the registry must signal the caller to
// repair via snapshot instead of silently diverging.
func TestRegistry_SequenceGapDetection(t *testing.T) {
	r := NewRegistry(testLogger())

	// Unknown owner whose numbering does not start at 1: this pod missed
	// earlier deltas (e.g. it joined late) and must snapshot.
	if r.ApplyDelta(addDelta("gw-1", 5, "dataplane/dp-a", "conn-1")) {
		t.Fatal("expected gap for unknown owner with seq != 1")
	}

	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-1")) {
		t.Fatal("expected seq 1 to apply")
	}
	if r.ApplyDelta(addDelta("gw-1", 3, "dataplane/dp-a", "conn-2")) {
		t.Fatal("expected gap when a sequence number is skipped")
	}
	// Stale entries survive the gap so routing continues until the snapshot
	// arrives.
	if got := r.Lookup("dataplane/dp-a", ""); len(got) != 1 {
		t.Fatalf("expected stale entry to remain during repair, got %+v", got)
	}

	// An owner restart resets its numbering; that must also read as a gap.
	if r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-9")) {
		t.Fatal("expected gap when owner sequence resets")
	}
}

// Snapshots replace an owner's state wholesale — the repair path for gaps and
// the bootstrap path for a joining pod.
func TestRegistry_SnapshotReplacesOwnerState(t *testing.T) {
	r := NewRegistry(testLogger())
	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "stale-conn")) {
		t.Fatal("setup delta failed")
	}

	r.ApplySnapshot(Snapshot{
		Owner: "gw-1",
		Seq:   7,
		Entries: []AgentEntry{
			{PlaneIdentifier: "dataplane/dp-a", ConnID: "conn-a"},
			{PlaneIdentifier: "dataplane/dp-b", ConnID: "conn-b"},
		},
	})

	agents := r.Lookup("dataplane/dp-a", "")
	if len(agents) != 1 || agents[0].Entry.ConnID != "conn-a" {
		t.Fatalf("expected snapshot to replace stale entries, got %+v", agents)
	}

	// Delta numbering continues from the snapshot's sequence.
	if !r.ApplyDelta(addDelta("gw-1", 8, "dataplane/dp-c", "conn-c")) {
		t.Fatal("expected delta continuing from snapshot seq to apply")
	}
}

// Ownership dies with the owner: a pod gone from EndpointSlices loses all its
// entries at once, with no tombstones.
func TestRegistry_PurgeOwner(t *testing.T) {
	r := NewRegistry(testLogger())
	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-1")) {
		t.Fatal("setup delta failed")
	}
	if !r.ApplyDelta(addDelta("gw-2", 1, "dataplane/dp-a", "conn-2")) {
		t.Fatal("setup delta failed")
	}

	r.PurgeOwner("gw-1")

	agents := r.Lookup("dataplane/dp-a", "")
	if len(agents) != 1 || agents[0].Owner != "gw-2" {
		t.Fatalf("expected only gw-2 entries to survive purge, got %+v", agents)
	}
}

// A draining pod's agents are still connected (status must count them) but
// must not receive new forwards (lookup must skip them).
func TestRegistry_DrainingExcludedFromLookupButCounted(t *testing.T) {
	r := NewRegistry(testLogger())
	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-1", "ns/cr-1")) {
		t.Fatal("setup delta failed")
	}

	r.SetDraining("gw-1")

	if got := r.Lookup("dataplane/dp-a", ""); len(got) != 0 {
		t.Fatalf("expected draining owner excluded from lookup, got %+v", got)
	}
	if got := r.CountForPlane("dataplane/dp-a"); got != 1 {
		t.Fatalf("expected draining owner still counted for status, got %d", got)
	}
	if got := r.CountForCR("dataplane/dp-a", "ns/cr-1"); got != 1 {
		t.Fatalf("expected draining owner still counted for CR status, got %d", got)
	}
}

// Per-pod round-robin: successive lookups rotate the candidate order so load
// spreads across agents without cross-pod coordination.
func TestRegistry_LookupRotates(t *testing.T) {
	r := NewRegistry(testLogger())
	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-1")) {
		t.Fatal("setup delta failed")
	}
	if !r.ApplyDelta(addDelta("gw-2", 1, "dataplane/dp-a", "conn-2")) {
		t.Fatal("setup delta failed")
	}

	first := r.Lookup("dataplane/dp-a", "")[0]
	second := r.Lookup("dataplane/dp-a", "")[0]
	if first.Owner == second.Owner {
		t.Fatalf("expected rotation between owners, got %s twice", first.Owner)
	}
}

func TestRegistry_AllPlaneCounts(t *testing.T) {
	r := NewRegistry(testLogger())
	if !r.ApplyDelta(addDelta("gw-1", 1, "dataplane/dp-a", "conn-1")) {
		t.Fatal("setup delta failed")
	}
	if !r.ApplyDelta(addDelta("gw-1", 2, "dataplane/dp-a", "conn-2")) {
		t.Fatal("setup delta failed")
	}
	if !r.ApplyDelta(addDelta("gw-2", 1, "workflowplane/wp-1", "conn-3")) {
		t.Fatal("setup delta failed")
	}

	counts := r.AllPlaneCounts()
	if counts["dataplane/dp-a"] != 2 || counts["workflowplane/wp-1"] != 1 {
		t.Fatalf("unexpected plane counts: %+v", counts)
	}
}

// Rebalancing decisions are made from this count, so a draining owner must be
// excluded: its connections are already leaving, and counting them would make
// a healthy pod look like it holds less than its share.
func TestRegistry_CountByOwnerSkipsDraining(t *testing.T) {
	r := NewRegistry(testLogger())

	for _, d := range []Delta{
		{Op: DeltaOpAdd, Owner: "gw-a", Seq: 1, Entry: AgentEntry{PlaneIdentifier: "dataplane/p", ConnID: "a1"}},
		{Op: DeltaOpAdd, Owner: "gw-a", Seq: 2, Entry: AgentEntry{PlaneIdentifier: "dataplane/p", ConnID: "a2"}},
		{Op: DeltaOpAdd, Owner: "gw-b", Seq: 1, Entry: AgentEntry{PlaneIdentifier: "dataplane/p", ConnID: "b1"}},
	} {
		if !r.ApplyDelta(d) {
			t.Fatalf("delta %v rejected", d)
		}
	}

	counts := r.CountByOwner()
	if counts["gw-a"] != 2 || counts["gw-b"] != 1 {
		t.Fatalf("unexpected counts: %v", counts)
	}

	r.SetDraining("gw-a")
	counts = r.CountByOwner()
	if _, present := counts["gw-a"]; present {
		t.Fatalf("draining owner must be excluded, got %v", counts)
	}
	if counts["gw-b"] != 1 {
		t.Fatalf("healthy owner miscounted: %v", counts)
	}
}

// A delta carrying an op this pod does not understand (an older replica meeting
// a newer one mid-rollout) must be skipped without desyncing the stream: the
// sequence still advances, so the deltas behind it keep applying instead of
// every one of them being rejected as a gap.
func TestRegistry_UnknownDeltaOpIsSkippedWithoutDesync(t *testing.T) {
	r := NewRegistry(testLogger())

	unknown := addDelta("gw-a", 1, "dataplane/prod", "c1", "ns/dp1")
	unknown.Op = "teleport"
	if !r.ApplyDelta(unknown) {
		t.Fatal("an unknown op was treated as a sequence gap; it must only be skipped")
	}
	if got := r.Lookup("dataplane/prod", "ns/dp1"); len(got) != 0 {
		t.Fatalf("an unknown op was acted on: %+v", got)
	}

	// The next delta must still line up, proving the sequence was not desynced.
	if !r.ApplyDelta(addDelta("gw-a", 2, "dataplane/prod", "c2", "ns/dp1")) {
		t.Fatal("the delta after an unknown op was rejected: the sequence desynced")
	}
	if got := r.Lookup("dataplane/prod", "ns/dp1"); len(got) != 1 {
		t.Fatalf("got %d agents after the follow-up delta, want 1", len(got))
	}
}

// Rotation is only fair if it rotates a stable list. Two connections owned by
// the same pod arrive from a map, so without the connection ID tiebreak their
// order varies per call and the cursor lands on an arbitrary one each time.
func TestRegistry_LookupOrdersOneOwnersConnectionsByID(t *testing.T) {
	r := NewRegistry(testLogger())
	if !r.ApplyDelta(addDelta("gw-a", 1, "dataplane/prod", "conn-b", "ns/dp1")) {
		t.Fatal("failed to seed conn-b")
	}
	if !r.ApplyDelta(addDelta("gw-a", 2, "dataplane/prod", "conn-a", "ns/dp1")) {
		t.Fatal("failed to seed conn-a")
	}

	// Every call rotates, so over one full cycle each connection must lead
	// exactly once - the property the tiebreak exists to guarantee.
	leaders := map[string]int{}
	for range 4 {
		got := r.Lookup("dataplane/prod", "ns/dp1")
		if len(got) != 2 {
			t.Fatalf("got %d agents, want 2", len(got))
		}
		leaders[got[0].Entry.ConnID]++
	}
	if leaders["conn-a"] != 2 || leaders["conn-b"] != 2 {
		t.Fatalf("rotation was uneven over two cycles: %v; the order is not stable", leaders)
	}
}
