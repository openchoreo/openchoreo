// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"context"
	"log/slog"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// mockEventHandler is a mock implementation of EventHandler for testing.
type mockEventHandler struct {
	handleCalled bool
	handledEvent *corev1.Event
	callCount    int
	events       []*corev1.Event
}

func (m *mockEventHandler) HandleEvent(ctx context.Context, ev *corev1.Event) {
	m.handleCalled = true
	m.handledEvent = ev
	m.callCount++
	m.events = append(m.events, ev)
}

func TestNew(t *testing.T) {
	handler := &mockEventHandler{}
	logger := testLogger()

	// Test with nil clientset to verify initialization logic
	informer := New(nil, handler, logger)

	if informer == nil {
		t.Fatal("New() returned nil")
	}
	if informer.handler == nil {
		t.Error("handler not set correctly")
	}
	if informer.logger == nil {
		t.Error("logger not set correctly")
	}

	// Test with mock clientset
	var mockClientset *kubernetes.Clientset
	informer2 := New(mockClientset, handler, logger)
	if informer2 == nil {
		t.Fatal("New() with clientset returned nil")
	}
	if informer2.clientset != mockClientset {
		t.Error("clientset not set correctly")
	}
	if informer2.handler != handler {
		t.Error("handler not set correctly")
	}
	if informer2.logger != logger {
		t.Error("logger not set correctly")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
