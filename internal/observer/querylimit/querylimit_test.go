// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package querylimit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// requireLimited asserts err is a *LimitedError for reason and returns its
// RetryAfter.
func requireLimited(t *testing.T, err error, reason Reason) time.Duration {
	t.Helper()
	var limited *LimitedError
	require.True(t, errors.As(err, &limited), "want a *LimitedError, got %v", err)
	assert.Equal(t, reason, limited.Reason)
	return limited.RetryAfter
}

func TestNew_NoLimitsReturnsNil(t *testing.T) {
	assert.Nil(t, New(Config{}))
	assert.Nil(t, New(Config{Burst: 5}))

	var l *Limiter
	release, err := l.Acquire("anyone")
	require.NoError(t, err)
	release()
}

func TestAcquire_Concurrency(t *testing.T) {
	l := New(Config{MaxConcurrent: 2})

	r1, err := l.Acquire("a")
	require.NoError(t, err)
	r2, err := l.Acquire("b")
	require.NoError(t, err)

	_, err = l.Acquire("c")
	assert.Equal(t, time.Second, requireLimited(t, err, ReasonConcurrency))

	r1()
	r1() // releasing twice frees one slot only
	r3, err := l.Acquire("c")
	require.NoError(t, err)

	_, err = l.Acquire("d")
	_ = requireLimited(t, err, ReasonConcurrency)

	r2()
	r3()
	assert.Empty(t, l.slots)
}

func TestAcquire_RatePerCaller(t *testing.T) {
	clock := newFakeClock()
	l := newWithClock(Config{RequestsPerMinute: 10, Burst: 2}, clock.now)

	for range 2 {
		release, err := l.Acquire("alice")
		require.NoError(t, err)
		release()
	}
	_, err := l.Acquire("alice")
	// 10 per minute is one token every 6s, and the bucket is empty.
	assert.Equal(t, 6*time.Second, requireLimited(t, err, ReasonRate))

	// Another caller has a bucket of its own.
	_, err = l.Acquire("bob")
	require.NoError(t, err)

	// A refused attempt takes no token: after 4s, 2s remain.
	clock.advance(4 * time.Second)
	_, err = l.Acquire("alice")
	assert.Equal(t, 2*time.Second, requireLimited(t, err, ReasonRate))

	clock.advance(2 * time.Second)
	_, err = l.Acquire("alice")
	require.NoError(t, err)
}

func TestAcquire_ReadsTheClockUnderTheLock(t *testing.T) {
	// Read outside the lock, a request could reach a bucket with an earlier
	// time than the last one and rewind it, so the refill since then would be
	// counted twice.
	clock := newFakeClock()
	var l *Limiter
	l = newWithClock(Config{RequestsPerMinute: 10, Burst: 2}, func() time.Time {
		if l != nil && l.mu.TryLock() {
			l.mu.Unlock()
			t.Error("the clock was read without holding the lock")
		}
		return clock.now()
	})

	release, err := l.Acquire("alice")
	require.NoError(t, err)
	release()
}

func TestAcquire_RateBurstFloor(t *testing.T) {
	l := newWithClock(Config{RequestsPerMinute: 60}, newFakeClock().now)

	_, err := l.Acquire("alice")
	require.NoError(t, err)
	_, err = l.Acquire("alice")
	_ = requireLimited(t, err, ReasonRate)
}

func TestAcquire_RateRefusalFreesTheSlot(t *testing.T) {
	l := newWithClock(Config{MaxConcurrent: 1, RequestsPerMinute: 1, Burst: 1}, newFakeClock().now)

	release, err := l.Acquire("alice")
	require.NoError(t, err)
	release()

	_, err = l.Acquire("alice")
	_ = requireLimited(t, err, ReasonRate)

	// The refused query gave its slot back.
	_, err = l.Acquire("bob")
	require.NoError(t, err)
}

func TestAcquire_ConcurrencyRefusalTakesNoToken(t *testing.T) {
	l := newWithClock(Config{MaxConcurrent: 1, RequestsPerMinute: 1, Burst: 1}, newFakeClock().now)

	release, err := l.Acquire("alice")
	require.NoError(t, err)

	_, err = l.Acquire("bob")
	_ = requireLimited(t, err, ReasonConcurrency)

	release()
	_, err = l.Acquire("bob")
	require.NoError(t, err)
}

func TestAcquire_RateRefusalWhileSlotsAreFull(t *testing.T) {
	// A caller over its rate gets its real wait, not the concurrency hint, so
	// it doesn't retry every second while the slots are full.
	l := newWithClock(Config{MaxConcurrent: 1, RequestsPerMinute: 10, Burst: 1}, newFakeClock().now)

	_, err := l.Acquire("alice")
	require.NoError(t, err)

	_, err = l.Acquire("alice")
	assert.Equal(t, 6*time.Second, requireLimited(t, err, ReasonRate))
}

func TestAcquire_RateRefusalNeverHoldsASlot(t *testing.T) {
	// The rate is checked before a slot is taken, so a caller over its rate
	// can't hold the last free slot for a moment and get another caller's
	// query refused for concurrency.
	clock := newFakeClock()
	var l *Limiter
	var slotsHeldAtRateCheck []int
	l = newWithClock(Config{MaxConcurrent: 2, RequestsPerMinute: 10, Burst: 1}, func() time.Time {
		if l != nil {
			slotsHeldAtRateCheck = append(slotsHeldAtRateCheck, len(l.slots))
		}
		return clock.now()
	})

	release, err := l.Acquire("alice")
	require.NoError(t, err)
	release()

	_, err = l.Acquire("alice")
	_ = requireLimited(t, err, ReasonRate)
	assert.Equal(t, []int{0, 0}, slotsHeldAtRateCheck)
}

func TestAcquire_DropsIdleCallers(t *testing.T) {
	clock := newFakeClock()
	// A full refill takes burst / rate = 2 / (1 per 6s) = 12s.
	l := newWithClock(Config{RequestsPerMinute: 10, Burst: 2}, clock.now)

	_, err := l.Acquire("alice")
	require.NoError(t, err)
	_, err = l.Acquire("bob")
	require.NoError(t, err)
	assert.Len(t, l.callers, 2)

	// Sweeps run at most once a minute: at 55s alice is idle, but still kept.
	clock.advance(55 * time.Second)
	_, err = l.Acquire("bob")
	require.NoError(t, err)
	assert.Len(t, l.callers, 2)

	// At 61s, alice (idle 61s) is dropped; bob (idle 6s) is kept.
	clock.advance(6 * time.Second)
	_, err = l.Acquire("carol")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"bob", "carol"}, keys(l.callers))

	// A minute later bob is idle past the 12s refill; carol was just seen.
	clock.advance(sweepInterval)
	_, err = l.Acquire("carol")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"carol"}, keys(l.callers))
}

func keys(m map[string]*caller) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestAcquire_ParallelNeverExceedsMaxConcurrent(t *testing.T) {
	const maxConcurrent = 3
	l := New(Config{MaxConcurrent: maxConcurrent, RequestsPerMinute: 6000, Burst: 1000})

	var running, peak, admitted atomic.Int32
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.Acquire([]string{"a", "b", "c"}[i%3])
			if err != nil {
				return
			}
			defer release()
			admitted.Add(1)
			n := running.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			running.Add(-1)
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, peak.Load(), int32(maxConcurrent))
	assert.Positive(t, admitted.Load())
	assert.Empty(t, l.slots)
}

func TestLimitedError_Error(t *testing.T) {
	assert.Equal(t, "too many queries are running", (&LimitedError{Reason: ReasonConcurrency}).Error())
	assert.Equal(t, "too many queries from this caller; retry after 6s",
		(&LimitedError{Reason: ReasonRate, RetryAfter: 6 * time.Second}).Error())
}

func TestCallerKey(t *testing.T) {
	assert.Empty(t, CallerKey(context.Background()))
	assert.Empty(t, CallerKey(auth.SetSubjectContext(context.Background(), &auth.SubjectContext{Issuer: "idp"})))

	alice := auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID: "alice", Issuer: "idp-a", ReadableID: "alice@example.com",
	})
	aliceOtherIssuer := auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID: "alice", Issuer: "idp-b", ReadableID: "alice@example.com",
	})
	assert.NotEmpty(t, CallerKey(alice))
	assert.NotEqual(t, CallerKey(alice), CallerKey(aliceOtherIssuer))
}
