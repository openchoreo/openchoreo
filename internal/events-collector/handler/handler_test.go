// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// mockCheckpointStore is a mock implementation of CheckpointStore for testing.
type mockCheckpointStore struct {
	addResult bool
	addError  error
	addCalled bool
	addUID    string
}

func (m *mockCheckpointStore) Add(eventUID string) (bool, error) {
	m.addCalled = true
	m.addUID = eventUID
	return m.addResult, m.addError
}

// mockLabelResolver is a mock implementation of LabelResolver for testing.
type mockLabelResolver struct {
	labels        map[string]string
	resolveError  error
	resolveCalled bool
	resolvedObj   corev1.ObjectReference
}

func (m *mockLabelResolver) Resolve(ctx context.Context, obj corev1.ObjectReference) (map[string]string, error) {
	m.resolveCalled = true
	m.resolvedObj = obj
	return m.labels, m.resolveError
}

func TestFormatTimestamp(t *testing.T) {
	// Reference time for testing
	refTime := time.Date(2025, 1, 15, 10, 30, 45, 0, time.UTC)
	expected := "2025-01-15T10:30:45Z"

	tests := []struct {
		name     string
		event    *corev1.Event
		expected string
	}{
		{
			name: "uses FirstTimestamp when set",
			event: &corev1.Event{
				FirstTimestamp: metav1.NewTime(refTime),
				EventTime:      metav1.NewMicroTime(refTime.Add(1 * time.Hour)),
				ObjectMeta:     metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(refTime.Add(2 * time.Hour))},
			},
			expected: expected,
		},
		{
			name: "falls back to EventTime when FirstTimestamp is zero",
			event: &corev1.Event{
				FirstTimestamp: metav1.Time{},
				EventTime:      metav1.NewMicroTime(refTime),
				ObjectMeta:     metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(refTime.Add(1 * time.Hour))},
			},
			expected: expected,
		},
		{
			name: "falls back to CreationTimestamp when both are zero",
			event: &corev1.Event{
				FirstTimestamp: metav1.Time{},
				EventTime:      metav1.MicroTime{},
				ObjectMeta:     metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(refTime)},
			},
			expected: expected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimestamp(tt.event)
			if got != tt.expected {
				t.Errorf("formatTimestamp() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatLastTimestamp(t *testing.T) {
	refTime := time.Date(2025, 1, 15, 11, 45, 30, 0, time.UTC)

	tests := []struct {
		name     string
		event    *corev1.Event
		expected string
	}{
		{
			name: "returns formatted time when LastTimestamp is set",
			event: &corev1.Event{
				LastTimestamp: metav1.NewTime(refTime),
			},
			expected: "2025-01-15T11:45:30Z",
		},
		{
			name: "returns empty string when LastTimestamp is zero",
			event: &corev1.Event{
				LastTimestamp: metav1.Time{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLastTimestamp(tt.event)
			if got != tt.expected {
				t.Errorf("formatLastTimestamp() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestHandleEvent_NewEvent(t *testing.T) {
	mockStore := &mockCheckpointStore{
		addResult: true, // New event
		addError:  nil,
	}
	mockResolver := &mockLabelResolver{
		labels: map[string]string{
			"openchoreo.dev/component": "my-component",
		},
	}

	handler := New(mockStore, mockResolver, testLogger())

	event := createTestEvent("test-event-1", "default", "my-pod")

	handler.HandleEvent(context.Background(), event)

	// Verify resolver was called
	if !mockResolver.resolveCalled {
		t.Error("LabelResolver.Resolve() was not called")
	}
	if mockResolver.resolvedObj.Name != "my-pod" {
		t.Errorf("Resolved wrong object: got %q, want %q", mockResolver.resolvedObj.Name, "my-pod")
	}
	if mockResolver.resolvedObj.Namespace != "default" {
		t.Errorf("Resolved wrong namespace: got %q, want %q", mockResolver.resolvedObj.Namespace, "default")
	}

	// Verify checkpoint store was called
	if !mockStore.addCalled {
		t.Error("CheckpointStore.Add() was not called")
	}
	if mockStore.addUID != "test-event-1" {
		t.Errorf("Added wrong UID: got %q, want %q", mockStore.addUID, "test-event-1")
	}
}

func TestHandleEvent_DuplicateEvent(t *testing.T) {
	mockStore := &mockCheckpointStore{
		addResult: false, // Duplicate event
		addError:  nil,
	}
	mockResolver := &mockLabelResolver{
		labels: map[string]string{},
	}

	handler := New(mockStore, mockResolver, testLogger())

	event := createTestEvent("duplicate-event", "kube-system", "kube-proxy")

	handler.HandleEvent(context.Background(), event)

	// Verify resolver and store were both called
	if !mockResolver.resolveCalled {
		t.Error("LabelResolver.Resolve() should still be called for duplicates")
	}
	if !mockStore.addCalled {
		t.Error("CheckpointStore.Add() was not called")
	}
	// For duplicates, the handler should skip emitting the event (we can't easily verify this
	// without capturing stdout, but we at least verify the flow reached the checkpoint)
}

func TestHandleEvent_CheckpointError(t *testing.T) {
	mockStore := &mockCheckpointStore{
		addResult: false,
		addError:  errors.New("database error"),
	}
	mockResolver := &mockLabelResolver{
		labels: map[string]string{},
	}

	handler := New(mockStore, mockResolver, testLogger())

	event := createTestEvent("error-event", "production", "web-server")

	// Should not panic
	handler.HandleEvent(context.Background(), event)

	// Verify store was called
	if !mockStore.addCalled {
		t.Error("CheckpointStore.Add() was not called")
	}
}

func TestHandleEvent_LabelResolutionError(t *testing.T) {
	mockStore := &mockCheckpointStore{
		addResult: true,
		addError:  nil,
	}
	mockResolver := &mockLabelResolver{
		labels:       nil,
		resolveError: errors.New("failed to resolve labels"),
	}

	handler := New(mockStore, mockResolver, testLogger())

	event := createTestEvent("label-error-event", "staging", "api-gateway")

	// Should continue processing without labels
	handler.HandleEvent(context.Background(), event)

	// Verify both resolver and store were called
	if !mockResolver.resolveCalled {
		t.Error("LabelResolver.Resolve() was not called")
	}
	if !mockStore.addCalled {
		t.Error("CheckpointStore.Add() was not called - handler should continue despite label error")
	}
}

func TestHandleEvent_EnrichedEventFields(t *testing.T) {
	// This test verifies that the handler correctly extracts fields from the K8s event
	// We can't easily verify the JSON output without refactoring, but we verify
	// that the resolver receives the correct object reference

	mockStore := &mockCheckpointStore{
		addResult: true,
		addError:  nil,
	}
	mockResolver := &mockLabelResolver{
		labels: map[string]string{
			"openchoreo.dev/project":     "test-project",
			"openchoreo.dev/environment": "staging",
		},
	}

	handler := New(mockStore, mockResolver, testLogger())

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
			UID:       types.UID("event-uid-123"),
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion:      "apps/v1",
			Kind:            "Deployment",
			Name:            "my-deployment",
			Namespace:       "production",
			ResourceVersion: "12345",
			UID:             types.UID("dep-uid-456"),
		},
		Reason:         "ScalingReplicaSet",
		Message:        "Scaled up replica set my-deployment-abc to 3",
		Type:           "Normal",
		FirstTimestamp: metav1.NewTime(time.Now()),
	}

	handler.HandleEvent(context.Background(), event)

	// Verify the involved object was correctly passed to resolver
	if mockResolver.resolvedObj.Kind != "Deployment" {
		t.Errorf("Expected Kind 'Deployment', got %q", mockResolver.resolvedObj.Kind)
	}
	if mockResolver.resolvedObj.Namespace != "production" {
		t.Errorf("Expected Namespace 'production', got %q", mockResolver.resolvedObj.Namespace)
	}
	if mockResolver.resolvedObj.Name != "my-deployment" {
		t.Errorf("Expected Name 'my-deployment', got %q", mockResolver.resolvedObj.Name)
	}
}

// createTestEvent creates a basic K8s Event for testing.
func createTestEvent(uid, namespace, podName string) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       podName,
			Namespace:  namespace,
		},
		Reason:         "TestReason",
		Message:        "Test message",
		Type:           "Normal",
		FirstTimestamp: metav1.NewTime(time.Now()),
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
