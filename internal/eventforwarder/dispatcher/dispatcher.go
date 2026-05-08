// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/eventforwarder/config"
)

// Event represents a lightweight CRD change notification.
type Event struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Action    string `json:"action"`
}

// Dispatcher sends webhook notifications to configured HTTP endpoints.
type Dispatcher struct {
	endpoints []config.EndpointConfig
	client    *http.Client
	logger    *slog.Logger
}

// New creates a new Dispatcher.
func New(cfg config.WebhooksConfig, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		endpoints: cfg.Endpoints,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Dispatch sends an event to all configured endpoints.
//
// Lifecycle: spawns one parent goroutine per call, which in turn spawns
// one child goroutine per configured endpoint and waits for all of them
// via a WaitGroup. The caller (informer event handler) does NOT wait on
// the parent — blocking the informer would stall every CRD event in the
// process. The parent is "fire-and-forget at the informer boundary" but
// owns a clean per-event lifecycle internally, so when ctx is cancelled
// (SIGTERM) all in-flight retry sleeps and HTTP calls abort together
// instead of running detached past process shutdown.
//
// Note: this does NOT bound peak goroutine count under burst load — that
// requires a worker pool. Tracked separately as a follow-up.
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

	go d.dispatchAll(ctx, payload, event)
}

// dispatchAll fans out one HTTP delivery per configured endpoint and
// waits for all of them to finish (or for ctx to cancel them).
func (d *Dispatcher) dispatchAll(ctx context.Context, payload []byte, event Event) {
	var wg sync.WaitGroup
	for _, ep := range d.endpoints {
		wg.Add(1)
		go func(ep config.EndpointConfig) {
			defer wg.Done()
			d.sendWithRetry(ctx, ep, payload, event)
		}(ep)
	}
	wg.Wait()
	d.logger.Debug("All endpoint dispatches complete for event",
		"kind", event.Kind,
		"name", event.Name,
		"action", event.Action,
	)
}

func (d *Dispatcher) sendWithRetry(ctx context.Context, ep config.EndpointConfig, payload []byte, event Event) {
	url := ep.URL
	// Default behaviour is "try once and give up" — Backstage and similar
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
			d.logger.Info("Dispatch cancelled before attempt",
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
			d.logger.Info("Dispatch cancelled during attempt",
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
				d.logger.Info("Dispatch cancelled during backoff",
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

	return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}
