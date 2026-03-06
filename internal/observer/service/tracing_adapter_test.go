// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"
)

func TestNewTracingAdapter_DefaultTimeout(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 0, // Should use default
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewTracingAdapter_CustomTimeout(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 60 * time.Second,
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewTracingAdapter_BaseURLSet(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://traces-adapter.example.com:9000",
		Timeout: 30 * time.Second,
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewTracingAdapter_ClientInitialized(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 30 * time.Second,
	}

	adapter, err := NewTracingAdapter(config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.client == nil {
		t.Fatal("Expected non-nil client")
	}
}
