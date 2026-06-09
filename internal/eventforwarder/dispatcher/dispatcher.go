// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openchoreo/openchoreo/internal/eventforwarder/config"
)

// Event represents a lightweight Kubernetes resource change notification.
type Event struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Action    string `json:"action"`
}

// Default sizes for the worker pool. Worker count caps peak in-flight
// dispatches; queue size caps memory under burst when downstream is
// slow. With Backstage as the typical single consumer (sub-100ms
// responses) and the periodic full sync acting as the reconciliation
// safety net, modest values are sufficient — there's no benefit to
// over-provisioning, and dropping under sustained overload is honest
// behavior given the safety net.
const (
	defaultWorkers   = 4
	defaultQueueSize = 1024
)

// dispatchJob is the unit of work consumed by the worker pool. The
// payload is pre-marshaled at enqueue time so workers don't redo the
// JSON serialization, and ctx is captured at enqueue time so each
// in-flight job carries the producer's context for cancellation.
type dispatchJob struct {
	ctx     context.Context
	event   Event
	payload []byte
}

// Dispatcher sends webhook notifications to configured HTTP endpoints.
//
// Concurrency model: a fixed-size pool of worker goroutines consumes
// dispatch jobs from a buffered channel. The producer (`Dispatch`) is
// non-blocking — it never stalls the informer thread that's calling it,
// even under sustained downstream slowness. When the queue is full,
// new events are dropped with a warning; the consumer's periodic full
// sync is the reconciliation mechanism for any drops.
//
// This deliberately *does* bound peak goroutine count under burst load,
// at the cost of dropping events when the queue overflows. The
// previous design (one goroutine per event) had no such bound and could
// pile up arbitrarily during downstream outages.
type Dispatcher struct {
	endpoints []config.EndpointConfig
	client    *http.Client
	logger    *slog.Logger

	jobs    chan dispatchJob
	workers int
	started atomic.Bool
}

// New creates a new Dispatcher. Call Start to launch the worker pool
// before dispatching any events.
func New(cfg config.WebhooksConfig, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		endpoints: cfg.Endpoints,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:  logger,
		jobs:    make(chan dispatchJob, defaultQueueSize),
		workers: defaultWorkers,
	}
}

// Start launches the worker pool. The workers consume from the dispatch
// queue until ctx is canceled, at which point they drain any in-flight
// HTTP attempts (via the per-job ctx) and exit. Safe to call multiple
// times — only the first call has any effect.
func (d *Dispatcher) Start(ctx context.Context) {
	if !d.started.CompareAndSwap(false, true) {
		return
	}
	for i := 0; i < d.workers; i++ {
		go d.worker(ctx, i)
	}
	d.logger.Info("Dispatcher worker pool started",
		"workers", d.workers,
		"queueSize", cap(d.jobs),
	)
}

func (d *Dispatcher) worker(ctx context.Context, id int) {
	d.logger.Debug("Dispatcher worker started", "id", id)
	for {
		select {
		case <-ctx.Done():
			d.logger.Debug("Dispatcher worker stopping", "id", id, "reason", ctx.Err())
			return
		case job, ok := <-d.jobs:
			if !ok {
				return
			}
			d.dispatchAll(job)
		}
	}
}

// Dispatch enqueues an event for delivery. Returns immediately:
//   - If endpoints are empty, no-op.
//   - If the queue has capacity, the event is queued and the workers
//     will pick it up. The caller (informer event handler) is never
//     blocked.
//   - If the queue is full, the event is dropped with a warning. This
//     means the consumer is sustainedly slower than the event rate;
//     the periodic full sync will reconcile missed state.
func (d *Dispatcher) Dispatch(ctx context.Context, event Event) {
	if len(d.endpoints) == 0 {
		d.logger.Debug("No webhook endpoints configured, skipping dispatch",
			"kind", event.Kind,
			"name", event.Name,
		)
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		d.logger.Error("Failed to marshal event", "error", err)
		return
	}

	select {
	case d.jobs <- dispatchJob{ctx: ctx, event: event, payload: payload}:
		// queued; worker will pick it up
	default:
		d.logger.Warn("Dispatch queue full, dropping event — consumer too slow; periodic full sync will reconcile",
			"kind", event.Kind,
			"name", event.Name,
			"namespace", event.Namespace,
			"action", event.Action,
			"queueCapacity", cap(d.jobs),
		)
	}
}

// dispatchAll fans out one HTTP delivery per configured endpoint and
// waits for all of them to finish (or for ctx to cancel them). Runs on
// a worker goroutine; per-endpoint sub-goroutines preserve concurrent
// fan-out for events with multiple endpoints.
func (d *Dispatcher) dispatchAll(job dispatchJob) {
	var wg sync.WaitGroup
	for _, ep := range d.endpoints {
		wg.Add(1)
		go func(ep config.EndpointConfig) {
			defer wg.Done()
			d.sendWithRetry(job.ctx, ep, job.payload, job.event)
		}(ep)
	}
	wg.Wait()
	d.logger.Debug("All endpoint dispatches complete for event",
		"kind", job.event.Kind,
		"name", job.event.Name,
		"action", job.event.Action,
	)
}

func (d *Dispatcher) sendWithRetry(ctx context.Context, ep config.EndpointConfig, payload []byte, event Event) {
	url := ep.URL
	// Default behavior is "try once and give up" — Backstage and similar
	// catalog consumers reconcile missed events via their own periodic
	// full sync, so the forwarder doesn't need delivery guarantees by
	// default. Endpoints that have no equivalent reconciliation can opt
	// in to retry by setting `retry` in their config block.
	maxAttempts := 1
	backoffMs := 0
	if ep.Retry != nil {
		maxAttempts = ep.Retry.MaxAttempts
		if maxAttempts < 1 {
			maxAttempts = 1
		}
		backoffMs = ep.Retry.BackoffMs
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			d.logger.Info("Dispatch canceled before attempt",
				"url", url,
				"kind", event.Kind,
				"name", event.Name,
				"attempt", attempt,
				"error", ctx.Err(),
			)
			return
		}

		err := d.send(ctx, url, payload)
		if err == nil {
			d.logger.Debug("Webhook dispatched successfully",
				"url", url,
				"kind", event.Kind,
				"name", event.Name,
				"action", event.Action,
				"attempt", attempt,
			)
			return
		}

		// If the failure was caused by ctx cancellation, don't bother
		// retrying or escalating — log at info and return cleanly.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			d.logger.Info("Dispatch canceled during attempt",
				"url", url,
				"kind", event.Kind,
				"name", event.Name,
				"attempt", attempt,
				"error", err,
			)
			return
		}

		d.logger.Warn("Webhook dispatch failed",
			"url", url,
			"kind", event.Kind,
			"name", event.Name,
			"attempt", attempt,
			"maxAttempts", maxAttempts,
			"error", err,
		)

		if attempt < maxAttempts {
			backoff := time.Duration(backoffMs) * time.Millisecond * time.Duration(math.Pow(2, float64(attempt-1)))
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				d.logger.Info("Dispatch canceled during backoff",
					"url", url,
					"kind", event.Kind,
					"name", event.Name,
					"error", ctx.Err(),
				)
				return
			case <-timer.C:
			}
		}
	}

	d.logger.Error("Webhook dispatch failed after all retries",
		"url", url,
		"kind", event.Kind,
		"name", event.Name,
		"action", event.Action,
	)
}

func (d *Dispatcher) send(ctx context.Context, url string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Include a snippet of the response body in the error so the warn
	// log is actionable for troubleshooting (e.g. "404 Not Found", an
	// auth-rejection JSON, or a server-side stack trace excerpt). Cap
	// at maxBodySnippetBytes to keep log lines bounded regardless of
	// what the server returns.
	const maxBodySnippetBytes = 256
	bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodySnippetBytes))
	return fmt.Errorf("unexpected status code %d: %s",
		resp.StatusCode, strings.TrimSpace(string(bodySnippet)))
}
