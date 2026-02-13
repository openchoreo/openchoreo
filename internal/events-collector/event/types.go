// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package event

type EnrichedEvent struct {
	// RecordType identifies this log record as a Kubernetes event emitted by the
	// events-collector. It is used by log processors (e.g., fluent-bit) to
	// route and index these records separately from other container logs.
	RecordType string `json:"recordType"`

	// EnrichedEvent represents a Kubernetes event enriched with labels from its involved object.
	// This is the JSON structure that gets printed to stdout and picked up by fluent-bit.
	FirstTimestamp string         `json:"firstTimestamp"`
	LastTimestamp  string         `json:"lastTimestamp,omitempty"`
	Message        string         `json:"message"`
	Reason         string         `json:"reason"`
	Type           string         `json:"type"`
	InvolvedObject InvolvedObject `json:"involvedObject"`
}

// InvolvedObject represents the Kubernetes object that the event is about,
// enriched with its labels for filtering by OpenChoreo component/project/environment.
type InvolvedObject struct {
	APIVersion      string            `json:"apiVersion"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	ResourceVersion string            `json:"resourceVersion"`
	UID             string            `json:"uid"`
	Labels          map[string]string `json:"labels,omitempty"`
}
