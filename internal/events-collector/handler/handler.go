// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/events-collector/event"
)

// CheckpointStore provides event deduplication via checkpoint tracking.
type CheckpointStore interface {
	// Add adds an event UID to the checkpoint store.
	// Returns true if the event was newly inserted, false if it already existed.
	Add(eventUID string) (bool, error)
}

// LabelResolver resolves labels for Kubernetes objects.
type LabelResolver interface {
	// Resolve returns the labels for the given involved object.
	Resolve(ctx context.Context, involvedObj corev1.ObjectReference) (map[string]string, error)
}

// Handler processes Kubernetes events by deduplicating, enriching with labels,
// and logging the enriched events as JSON to stdout.
type Handler struct {
	checkpointStore CheckpointStore
	labelResolver   LabelResolver
	logger          *slog.Logger
}

// New creates a new event handler.
func New(store CheckpointStore, resolver LabelResolver, logger *slog.Logger) *Handler {
	return &Handler{
		checkpointStore: store,
		labelResolver:   resolver,
		logger:          logger,
	}
}

// HandleEvent processes a single Kubernetes event:
// 1. Resolves labels for the event's involved object.
// 2. Builds an enriched event struct.
// 3. Atomically adds the event to the checkpoint DB - if already processed, skips it.
// 4. Logs the enriched event as JSON to stdout.
func (h *Handler) HandleEvent(ctx context.Context, ev *corev1.Event) {
	eventUID := string(ev.UID)

	// Step 1: Resolve labels for the involved object
	labels, err := h.labelResolver.Resolve(ctx, ev.InvolvedObject)
	if err != nil {
		h.logger.Error("failed to resolve labels",
			"event_uid", eventUID,
			"involved_object", fmt.Sprintf("%s/%s", ev.InvolvedObject.Kind, ev.InvolvedObject.Name),
			"error", err,
		)
		// Continue without labels rather than dropping the event
	}

	// Step 2: Build enriched event
	enriched := event.EnrichedEvent{
		RecordType:     "kube-event",
		FirstTimestamp: formatTimestamp(ev),
		LastTimestamp:  formatLastTimestamp(ev),
		Message:        ev.Message,
		Reason:         ev.Reason,
		Type:           ev.Type,
		InvolvedObject: event.InvolvedObject{
			APIVersion:      ev.InvolvedObject.APIVersion,
			Kind:            ev.InvolvedObject.Kind,
			Name:            ev.InvolvedObject.Name,
			Namespace:       ev.InvolvedObject.Namespace,
			ResourceVersion: ev.InvolvedObject.ResourceVersion,
			UID:             string(ev.InvolvedObject.UID),
			Labels:          labels,
		},
	}

	// Step 3: Atomically add to checkpoint DB before emitting
	inserted, err := h.checkpointStore.Add(eventUID)
	if err != nil {
		h.logger.Error("failed to add event to checkpoint", "event_uid", eventUID, "error", err)
		return
	}
	if !inserted {
		// Event was already processed (duplicate)
		return
	}

	// Step 4: Log enriched event as JSON
	jsonBytes, err := json.Marshal(enriched)
	if err != nil {
		h.logger.Error("failed to marshal enriched event", "event_uid", eventUID, "error", err)
		return
	}

	// Output the enriched event as a raw JSON log line.
	// This uses fmt.Println to write to stdout so that fluent-bit picks it up as-is.
	fmt.Println(string(jsonBytes))
}

// formatTimestamp returns the first timestamp of the event in RFC3339 format.
// It prefers FirstTimestamp, falls back to EventTime, then CreationTimestamp.
func formatTimestamp(ev *corev1.Event) string {
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	if ev.EventTime.Time != (corev1.Event{}).EventTime.Time {
		return ev.EventTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	return ev.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z")
}

// formatLastTimestamp returns the last timestamp if available.
func formatLastTimestamp(ev *corev1.Event) string {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	return ""
}
