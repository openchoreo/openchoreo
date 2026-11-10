// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package querylimit bounds how many queries of one kind the Observer runs at
// once and how often each caller may run them. Raw queries are not limited to
// a scope or a time window, so these limits, with the timeout and the response
// size limit, are what protect the backend and the query quotas it shares with
// every other Observer query. Each kind of raw query (traces, and later logs
// and metrics) has its own Limiter.
//
// The limits are kept in memory, so they apply per Observer replica.
package querylimit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Reason names the limit a query was over.
type Reason string

const (
	// ReasonConcurrency means the maximum number of queries were already running.
	ReasonConcurrency Reason = "concurrency"
	// ReasonRate means the caller ran more queries than its rate allows.
	ReasonRate Reason = "rate"
)

// concurrencyRetryAfter is the wait suggested when every slot is taken. A
// running query frees its slot when it ends, which can't be predicted, so this
// is only a hint.
const concurrencyRetryAfter = time.Second

// sweepInterval is how often idle callers are dropped, at most.
const sweepInterval = time.Minute

// LimitedError is returned by Acquire when a query is over a limit.
type LimitedError struct {
	Reason Reason
	// RetryAfter is how long the caller should wait before trying again, in
	// whole seconds, at least one.
	RetryAfter time.Duration
}

func (e *LimitedError) Error() string {
	switch e.Reason {
	case ReasonConcurrency:
		return "too many queries are running"
	case ReasonRate:
		return fmt.Sprintf("too many queries from this caller; retry after %s", e.RetryAfter)
	default:
		return "query limited"
	}
}

// Config sets the limits. A zero value turns that limit off.
type Config struct {
	// MaxConcurrent is the most queries that may run at once.
	MaxConcurrent int
	// RequestsPerMinute is the rate at which each caller may start queries.
	RequestsPerMinute int
	// Burst is how many queries a caller may start at once before the rate
	// applies. It is raised to 1 when a rate is set.
	Burst int
}

// Limiter enforces a Config. A nil *Limiter allows everything.
type Limiter struct {
	slots chan struct{} // nil when concurrency is not limited

	limit     rate.Limit // zero when the rate is not limited
	burst     int
	idleAfter time.Duration // a bucket idle this long is full again

	now func() time.Time

	mu        sync.Mutex
	callers   map[string]*caller
	lastSweep time.Time
}

type caller struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// New builds a Limiter for cfg. It returns nil when cfg sets no limit.
func New(cfg Config) *Limiter {
	return newWithClock(cfg, time.Now)
}

func newWithClock(cfg Config, now func() time.Time) *Limiter {
	if cfg.MaxConcurrent <= 0 && cfg.RequestsPerMinute <= 0 {
		return nil
	}
	l := &Limiter{now: now, lastSweep: now()}
	if cfg.MaxConcurrent > 0 {
		l.slots = make(chan struct{}, cfg.MaxConcurrent)
	}
	if cfg.RequestsPerMinute > 0 {
		l.limit = rate.Limit(float64(cfg.RequestsPerMinute) / 60)
		l.burst = max(cfg.Burst, 1)
		// A bucket refills one token per 1/limit seconds, so after this long
		// with no queries it is full, the same as a new one.
		l.idleAfter = time.Duration(float64(l.burst) / float64(l.limit) * float64(time.Second))
		l.callers = make(map[string]*caller)
	}
	return l
}

// Acquire admits one query from callerKey, or returns a *LimitedError. On
// success the caller must call release once the query has finished. Queries
// are never queued: a query over a limit is refused at once.
func (l *Limiter) Acquire(callerKey string) (release func(), err error) {
	if l == nil {
		return func() {}, nil
	}
	if l.limit <= 0 {
		if !l.takeSlot() {
			return nil, &LimitedError{Reason: ReasonConcurrency, RetryAfter: concurrencyRetryAfter}
		}
		return l.releaser(), nil
	}

	// With a rate set, both limits are checked under one lock and at one time.
	// A query over its caller's rate is refused before it takes a slot, so it
	// never holds one, even for a moment, and gets its real wait; a query
	// refused for concurrency uses no token.
	l.mu.Lock()
	defer l.mu.Unlock()

	// Read under the lock, so a bucket never sees time go backwards: the rate
	// limiter takes an earlier time as its new last update and would count the
	// time between the two again.
	now := l.now()
	l.sweep(now)
	c := l.callerAt(callerKey, now)
	if tokens := c.limiter.TokensAt(now); tokens < 1 {
		return nil, &LimitedError{Reason: ReasonRate, RetryAfter: l.rateRetryAfter(tokens)}
	}
	if !l.takeSlot() {
		return nil, &LimitedError{Reason: ReasonConcurrency, RetryAfter: concurrencyRetryAfter}
	}
	release = l.releaser()
	// The bucket had a token at this time and the lock is still held, so this
	// does not fail; if it ever did, the query is refused for its rate.
	if !c.limiter.AllowN(now, 1) {
		release()
		return nil, &LimitedError{Reason: ReasonRate, RetryAfter: l.rateRetryAfter(c.limiter.TokensAt(now))}
	}
	return release, nil
}

// takeSlot takes a concurrency slot without waiting and reports whether it
// got one. It always succeeds when concurrency is not limited.
func (l *Limiter) takeSlot() bool {
	if l.slots == nil {
		return true
	}
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// releaser returns the function that gives back a slot taken by takeSlot. It
// frees the slot once, however often it is called.
func (l *Limiter) releaser() func() {
	if l.slots == nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(func() { <-l.slots }) }
}

// callerAt returns callerKey's bucket, adding a full one for a new caller, and
// marks it seen at now. l.mu must be held.
func (l *Limiter) callerAt(callerKey string, now time.Time) *caller {
	c, ok := l.callers[callerKey]
	if !ok {
		c = &caller{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.callers[callerKey] = c
	}
	c.lastSeen = now
	return c
}

// rateRetryAfter is how long until a bucket holding tokens has one, in whole
// seconds as Retry-After carries, at least one. The tolerance keeps float
// error in the token count from adding a second.
func (l *Limiter) rateRetryAfter(tokens float64) time.Duration {
	seconds := math.Ceil((1-tokens)/float64(l.limit) - 1e-6)
	return time.Duration(max(seconds, 1)) * time.Second
}

// sweep drops callers whose bucket has refilled, at most once per
// sweepInterval, so the map holds only recent callers. l.mu must be held.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	for key, c := range l.callers {
		if now.Sub(c.lastSeen) >= l.idleAfter {
			delete(l.callers, key)
		}
	}
}

// CallerKey identifies the caller of ctx for per-caller limits: the subject's
// ID, which is unique only within its issuer, so the issuer is part of the key.
// The readable ID is never used, since its claim is operator-configurable.
// Requests without a subject, as when authentication is off, share one key.
func CallerKey(ctx context.Context) string {
	subject, ok := auth.GetSubjectContextFromContext(ctx)
	if !ok || subject == nil || subject.ID == "" {
		return ""
	}
	return subject.Issuer + "\x00" + subject.ID
}
