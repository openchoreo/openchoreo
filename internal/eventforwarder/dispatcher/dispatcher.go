// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
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
	retry     config.RetryConfig
	client    *http.Client
	logger    *slog.Logger
}

// New creates a new Dispatcher.
func New(cfg config.WebhooksConfig, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		endpoints: cfg.Endpoints,
		retry:     cfg.Retry,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Dispatch sends an event to all configured endpoints.
func (d *Dispatcher) Dispatch(event Event) {
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

	for _, ep := range d.endpoints {
		go d.sendWithRetry(ep.URL, payload, event)
	}
}

func (d *Dispatcher) sendWithRetry(url string, payload []byte, event Event) {
	maxAttempts := d.retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := d.send(url, payload)
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

		d.logger.Warn("Webhook dispatch failed",
			"url", url,
			"kind", event.Kind,
			"name", event.Name,
			"attempt", attempt,
			"maxAttempts", maxAttempts,
			"error", err,
		)

		if attempt < maxAttempts {
			backoff := time.Duration(d.retry.BackoffMs) * time.Millisecond * time.Duration(math.Pow(2, float64(attempt-1)))
			time.Sleep(backoff)
		}
	}

	d.logger.Error("Webhook dispatch failed after all retries",
		"url", url,
		"kind", event.Kind,
		"name", event.Name,
		"action", event.Action,
	)
}

func (d *Dispatcher) send(url string, payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
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
