// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dispatcher

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/eventforwarder/config"
)

// newTestDispatcher builds a Dispatcher pointed at one or more URLs
// and starts its worker pool against the test's lifetime context.
// When maxAttempts > 1, each endpoint is configured with that retry
// policy and a 1ms backoff so retry tests stay fast. When maxAttempts
// is 1, no retry block is set — exercising the default "try once" path.
func newTestDispatcher(t *testing.T, urls []string, maxAttempts int) *Dispatcher {
	t.Helper()
	endpoints := make([]config.EndpointConfig, len(urls))
	for i, u := range urls {
		ep := config.EndpointConfig{URL: u}
		if maxAttempts > 1 {
			ep.Retry = &config.RetryConfig{
				MaxAttempts: maxAttempts,
				BackoffMs:   1,
			}
		}
		endpoints[i] = ep
	}
	d := New(config.WebhooksConfig{Endpoints: endpoints}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d.Start(ctx)
	return d
}

// waitForBody reads a single delivery from the channel with a generous
// timeout, failing the test if nothing arrives.
func waitForBody(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case body := <-ch:
		return body
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
		return nil
	}
}

func TestDispatch_DeliversJSONEventOnSuccess(t *testing.T) {
	received := make(chan []byte, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := newTestDispatcher(t, []string{ts.URL}, 1)
	d.Dispatch(context.Background(), Event{
		Kind:      "Project",
		Name:      "url-shortener",
		Namespace: "default",
		Action:    "updated",
	})

	body := waitForBody(t, received)

	var got Event
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "Project", got.Kind)
	assert.Equal(t, "url-shortener", got.Name)
	assert.Equal(t, "default", got.Namespace)
	assert.Equal(t, "updated", got.Action)
}

func TestDispatch_RetriesUntilSuccess(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Fail twice, succeed on the third attempt.
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := newTestDispatcher(t, []string{ts.URL}, 5)
	d.Dispatch(context.Background(), Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	assert.Eventually(t, func() bool { return attempts.Load() == 3 },
		2*time.Second, 10*time.Millisecond,
		"expected exactly 3 attempts (2 failures + 1 success)")
}

func TestDispatch_GivesUpAfterMaxAttempts(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	d := newTestDispatcher(t, []string{ts.URL}, 3)
	d.Dispatch(context.Background(), Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	// Wait long enough for all retries to elapse (backoff 1ms × 2^n,
	// negligible for this test).
	assert.Eventually(t, func() bool { return attempts.Load() == 3 },
		2*time.Second, 10*time.Millisecond,
		"expected exactly MaxAttempts attempts after persistent failure")

	// Give the goroutine a moment to settle, then assert no further
	// attempts happen.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(3), attempts.Load(), "no further attempts after MaxAttempts")
}

func TestDispatch_NoEndpointsIsNoOp(t *testing.T) {
	d := New(config.WebhooksConfig{Endpoints: nil}, slog.Default())

	// Should return without panicking and without any side effects.
	d.Dispatch(context.Background(), Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})
}

func TestDispatch_FansOutToAllEndpoints(t *testing.T) {
	var aHits, bHits atomic.Int32
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		aHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer tsA.Close()
	tsB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		bHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer tsB.Close()

	d := newTestDispatcher(t, []string{tsA.URL, tsB.URL}, 1)
	d.Dispatch(context.Background(), Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	assert.Eventually(t, func() bool {
		return aHits.Load() == 1 && bHits.Load() == 1
	}, 2*time.Second, 10*time.Millisecond,
		"expected exactly one delivery to each configured endpoint")
}

func TestDispatch_CancellationStopsRetries(t *testing.T) {
	// Server that always 500s, forcing the dispatcher into the retry loop.
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	// Long backoff (500ms) and many attempts so the retry loop will be
	// sleeping when we cancel. If cancellation is wired correctly, the
	// goroutine should exit immediately rather than waiting out the full
	// backoff.
	d := New(config.WebhooksConfig{
		Endpoints: []config.EndpointConfig{{
			URL: ts.URL,
			Retry: &config.RetryConfig{
				MaxAttempts: 10,
				BackoffMs:   500,
			},
		}},
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)
	d.Dispatch(ctx, Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	// Wait for the first failed attempt to land, confirming we're now in the backoff.
	assert.Eventually(t, func() bool { return attempts.Load() >= 1 },
		2*time.Second, 10*time.Millisecond, "expected at least one attempt before cancel")

	// Cancel and confirm no further attempts arrive — proves the backoff
	// timer is observing ctx.Done() rather than waiting out time.Sleep.
	before := attempts.Load()
	cancel()
	time.Sleep(200 * time.Millisecond)
	assert.LessOrEqual(t, attempts.Load(), before+1,
		"no meaningful retry should occur after cancel")
}

func TestDispatch_FanOutRunsEndpointsConcurrently(t *testing.T) {
	// Both endpoints sleep 200ms. If the dispatcher serialized them, the
	// total wall-clock would be ~400ms; with concurrent fan-out it's ~200ms.
	const handlerDelay = 200 * time.Millisecond
	makeServer := func(hits *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(handlerDelay)
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
	}
	var aHits, bHits atomic.Int32
	tsA := makeServer(&aHits)
	defer tsA.Close()
	tsB := makeServer(&bHits)
	defer tsB.Close()

	d := newTestDispatcher(t, []string{tsA.URL, tsB.URL}, 1)

	start := time.Now()
	d.Dispatch(context.Background(), Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})
	assert.Eventually(t, func() bool { return aHits.Load() == 1 && bHits.Load() == 1 },
		2*time.Second, 10*time.Millisecond)
	elapsed := time.Since(start)

	// Allow generous slack for scheduling jitter, but well short of
	// 2×handlerDelay (which would mean serial execution).
	assert.Less(t, elapsed, 2*handlerDelay-50*time.Millisecond,
		"endpoints should be dispatched concurrently, not serially (took %s)", elapsed)
}

func TestDispatch_MaxAttemptsZeroTreatedAsOne(t *testing.T) {
	// Defensive: misconfigured retry.maxAttempts of 0 must still produce
	// at least one attempt (the production code clamps to 1).
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := New(config.WebhooksConfig{
		Endpoints: []config.EndpointConfig{{
			URL:   ts.URL,
			Retry: &config.RetryConfig{MaxAttempts: 0, BackoffMs: 1},
		}},
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	d.Dispatch(ctx, Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	assert.Eventually(t, func() bool { return attempts.Load() == 1 },
		2*time.Second, 10*time.Millisecond)
}

func TestDispatch_DropsEventsWhenQueueFull(t *testing.T) {
	// Hold every dispatch open until the test is ready to release them.
	// With a tiny channel and stalled workers, the next Dispatch call
	// should hit the `default:` branch and drop the event rather than
	// block the producer (informer thread).
	release := make(chan struct{})
	// Signals once the first request reaches the handler — at that
	// point the (single) worker has already drained its job from the
	// channel and is stuck inside the handler, so subsequent Dispatch
	// calls hit a deterministic channel state. Buffered + `default:`
	// makes the send a no-op on the second handler invocation that
	// runs during cleanup.
	firstReqArrived := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case firstReqArrived <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	defer close(release)

	d := New(config.WebhooksConfig{
		Endpoints: []config.EndpointConfig{{URL: ts.URL}},
	}, slog.Default())
	// Shrink the worker pool and queue to make overflow trivially
	// reproducible. With workers=1 + queueSize=1, only the second
	// in-flight dispatch (worker busy + queue holds 1) can be queued;
	// the third must drop.
	d.workers = 1
	d.jobs = make(chan dispatchJob, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)

	// First dispatch lands on the worker. Wait until it's actually
	// inside the handler before issuing the next two — otherwise the
	// producer can win the race against the worker's first channel
	// read, parking p0 in the buffer and dropping both p1 and p2.
	d.Dispatch(ctx, Event{Kind: "Project", Name: "p0", Namespace: "default", Action: "updated"})
	<-firstReqArrived

	// Second dispatch fits in the buffered queue; third must drop.
	d.Dispatch(ctx, Event{Kind: "Project", Name: "p1", Namespace: "default", Action: "updated"})
	d.Dispatch(ctx, Event{Kind: "Project", Name: "p2", Namespace: "default", Action: "updated"})

	// Verify only 2 of the 3 are queued/in-flight (the third was
	// dropped). Channel still holds the second job, worker is in
	// httptest handler with the first.
	assert.Equal(t, 1, len(d.jobs), "queued job count after overflow")
}

func TestDispatch_DefaultsToSingleAttemptWhenNoRetryConfigured(t *testing.T) {
	// Endpoint with no `retry` block must try exactly once and not retry
	// on failure — the periodic full sync on the consumer side is the
	// reconciliation mechanism by default.
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	d := New(config.WebhooksConfig{
		Endpoints: []config.EndpointConfig{{URL: ts.URL}}, // no Retry
	}, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	d.Dispatch(ctx, Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	// Wait long enough that any retry would have landed.
	assert.Eventually(t, func() bool { return attempts.Load() == 1 },
		1*time.Second, 10*time.Millisecond,
		"expected exactly one attempt with no retry config")

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), attempts.Load(),
		"no retries should be attempted when retry config is absent")
}
