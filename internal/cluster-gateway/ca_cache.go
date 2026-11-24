// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"sync"
	"time"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// caCacheTTL is how long a client CA read from a Secret is reused by agent
// connects.
const caCacheTTL = 5 * time.Minute

// secretCAKey identifies one CA bundle: a key of a Secret.
type secretCAKey struct{ namespace, name, key string }

// secretCAKeyFor returns the key a Secret reference resolves to. A reference
// without a namespace points into the namespace of the plane CR that holds it.
func secretCAKeyFor(ref *openchoreov1alpha1.SecretKeyReference, planeNamespace string) secretCAKey {
	namespace := ref.Namespace
	if namespace == "" {
		namespace = planeNamespace
	}
	return secretCAKey{namespace: namespace, name: ref.Name, key: ref.Key}
}

type secretCAEntry struct {
	data   []byte
	failed bool      // the read failed: there is no CA to reuse
	readAt time.Time // when the read that produced this entry started
}

// secretCACache holds client CA bundles read from Secrets, for agent connects
// to reuse. A connect checks every CR that shares its planeID, and connects
// come in groups: the replicas of one agent, or a reconnect after a dropped
// connection. Without the cache each of them reads every CR's Secret again.
//
// Every read of a CA Secret is recorded, including the ones a plane
// notification makes and the ones that fail, so an entry always reflects the
// read of its Secret that started last. An entry is used for caCacheTTL from
// that read. A CA changed only in its Secret, with no change to the plane CR
// and so no notification, is therefore seen by a new connection at most
// caCacheTTL late.
type secretCACache struct {
	mu       sync.Mutex
	entries  map[secretCAKey]secretCAEntry
	prunedAt time.Time
	now      func() time.Time
}

func newSecretCACache() *secretCACache {
	return &secretCACache{entries: make(map[secretCAKey]secretCAEntry), now: time.Now}
}

// get returns the CA recorded for key if it was read within caCacheTTL.
func (c *secretCACache) get(key secretCAKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok || e.failed || c.now().Sub(e.readAt) >= caCacheTTL {
		return nil, false
	}
	return e.data, true
}

// put records the CA returned by a read that started at readAt. Two reads of
// one Secret can finish in either order, and the one that started later saw the
// newer Secret, so a read older than the recorded one is dropped.
func (c *secretCACache) put(key secretCAKey, data []byte, readAt time.Time) {
	c.record(key, secretCAEntry{data: data, readAt: readAt})
}

// drop records that a read which started at readAt failed, so the CA recorded
// before it can no longer be relied on. The failure is kept as an entry of its
// own and not just removed: a read that started earlier and finishes later
// would otherwise find nothing recorded and put the old CA back.
func (c *secretCACache) drop(key secretCAKey, readAt time.Time) {
	c.record(key, secretCAEntry{failed: true, readAt: readAt})
}

func (c *secretCACache) record(key secretCAKey, entry secretCAEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[key]; ok && e.readAt.After(entry.readAt) {
		return
	}
	c.entries[key] = entry
	c.pruneLocked()
}

// pruneLocked removes expired entries, at most once per caCacheTTL. Entries are
// otherwise only ever replaced, so the Secrets of deleted plane CRs would stay
// for the life of the pod.
func (c *secretCACache) pruneLocked() {
	now := c.now()
	if now.Sub(c.prunedAt) < caCacheTTL {
		return
	}
	c.prunedAt = now
	for key, e := range c.entries {
		if now.Sub(e.readAt) >= caCacheTTL {
			delete(c.entries, key)
		}
	}
}
