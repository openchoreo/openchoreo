// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEnrichedEvent_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		event    EnrichedEvent
		expected map[string]interface{}
	}{
		{
			name: "full event with all fields",
			event: EnrichedEvent{
				RecordType:     "kube-event",
				FirstTimestamp: "2025-01-15T10:30:45Z",
				LastTimestamp:  "2025-01-15T11:00:00Z",
				Message:        "Pod started successfully",
				Reason:         "Started",
				Type:           "Normal",
				InvolvedObject: InvolvedObject{
					APIVersion:      "v1",
					Kind:            "Pod",
					Name:            "my-pod",
					Namespace:       "default",
					ResourceVersion: "12345",
					UID:             "pod-uid-123",
					Labels: map[string]string{
						"openchoreo.dev/component": "my-component",
					},
				},
			},
			expected: map[string]interface{}{
				"recordType":     "kube-event",
				"firstTimestamp": "2025-01-15T10:30:45Z",
				"lastTimestamp":  "2025-01-15T11:00:00Z",
				"message":        "Pod started successfully",
				"reason":         "Started",
				"type":           "Normal",
				"involvedObject": map[string]interface{}{
					"apiVersion":      "v1",
					"kind":            "Pod",
					"name":            "my-pod",
					"namespace":       "default",
					"resourceVersion": "12345",
					"uid":             "pod-uid-123",
					"labels": map[string]interface{}{
						"openchoreo.dev/component": "my-component",
					},
				},
			},
		},
		{
			name: "event with omitted optional fields",
			event: EnrichedEvent{
				RecordType:     "kube-event",
				FirstTimestamp: "2025-01-15T10:30:45Z",
				LastTimestamp:  "", // omitempty
				Message:        "Test message",
				Reason:         "TestReason",
				Type:           "Warning",
				InvolvedObject: InvolvedObject{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       "my-pod",
					Namespace:  "default",
					Labels:     nil, // omitempty
				},
			},
			expected: map[string]interface{}{
				"recordType":     "kube-event",
				"firstTimestamp": "2025-01-15T10:30:45Z",
				// lastTimestamp should be omitted
				"message": "Test message",
				"reason":  "TestReason",
				"type":    "Warning",
				"involvedObject": map[string]interface{}{
					"apiVersion":      "v1",
					"kind":            "Pod",
					"name":            "my-pod",
					"namespace":       "default",
					"resourceVersion": "",
					"uid":             "",
					// labels should be omitted
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			jsonBytes, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("json.Marshal() error: %v", err)
			}

			// Unmarshal back to map for comparison
			var got map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &got); err != nil {
				t.Fatalf("json.Unmarshal() error: %v", err)
			}

			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("JSON marshaling mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEnrichedEvent_JSONFieldNames(t *testing.T) {
	// Verify that JSON field names are correct (camelCase)
	event := EnrichedEvent{
		RecordType:     "test",
		FirstTimestamp: "ts",
		LastTimestamp:  "lts",
		Message:        "msg",
		Reason:         "reason",
		Type:           "type",
		InvolvedObject: InvolvedObject{
			APIVersion:      "v1",
			Kind:            "Pod",
			Name:            "name",
			Namespace:       "ns",
			ResourceVersion: "rv",
			UID:             "uid",
			Labels:          map[string]string{"k": "v"},
		},
	}

	jsonBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &got); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	// Check top-level field names
	expectedFields := []string{"recordType", "firstTimestamp", "lastTimestamp", "message", "reason", "type", "involvedObject"}
	for _, field := range expectedFields {
		if _, ok := got[field]; !ok {
			t.Errorf("Expected field %q not found in JSON", field)
		}
	}

	// Check involvedObject field names
	involvedObj, ok := got["involvedObject"].(map[string]interface{})
	if !ok {
		t.Fatal("involvedObject is not a map")
	}

	involvedObjFields := []string{"apiVersion", "kind", "name", "namespace", "resourceVersion", "uid", "labels"}
	for _, field := range involvedObjFields {
		if _, ok := involvedObj[field]; !ok {
			t.Errorf("Expected field involvedObject.%q not found in JSON", field)
		}
	}
}
