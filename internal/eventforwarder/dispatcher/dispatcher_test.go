// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dispatcher

import (
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

// newTestDispatcher builds a Dispatcher pointed at one or more URLs with
// fast retry settings so tests don't sleep on backoff.
func newTestDispatcher(urls []string, maxAttempts int) *Dispatcher {
	endpoints := make([]config.EndpointConfig, len(urls))
	for i, u := range urls {
		endpoints[i] = config.EndpointConfig{URL: u}
	}
	return New(config.WebhooksConfig{
		Endpoints: endpoints,
		Retry: config.RetryConfig{
			MaxAttempts: maxAttempts,
			BackoffMs:   1, // 1ms backoff keeps retry tests fast
		},
	}, slog.Default())
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

	d := newTestDispatcher([]string{ts.URL}, 1)
	d.Dispatch(Event{
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

	d := newTestDispatcher([]string{ts.URL}, 5)
	d.Dispatch(Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

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

	d := newTestDispatcher([]string{ts.URL}, 3)
	d.Dispatch(Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

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
	d := New(config.WebhooksConfig{
		Endpoints: nil,
		Retry:     config.RetryConfig{MaxAttempts: 3, BackoffMs: 1},
	}, slog.Default())

	// Should return without panicking and without any side effects.
	d.Dispatch(Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})
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

	d := newTestDispatcher([]string{tsA.URL, tsB.URL}, 1)
	d.Dispatch(Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	assert.Eventually(t, func() bool {
		return aHits.Load() == 1 && bHits.Load() == 1
	}, 2*time.Second, 10*time.Millisecond,
		"expected exactly one delivery to each configured endpoint")
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

	d := newTestDispatcher([]string{ts.URL}, 0)
	d.Dispatch(Event{Kind: "Project", Name: "foo", Namespace: "default", Action: "updated"})

	assert.Eventually(t, func() bool { return attempts.Load() == 1 },
		2*time.Second, 10*time.Millisecond)
}
