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

	adapter := NewTracingAdapter(config)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}

	if adapter.httpClient.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", adapter.httpClient.Timeout)
	}
}

func TestNewTracingAdapter_CustomTimeout(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 60 * time.Second,
	}

	adapter := NewTracingAdapter(config)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}

	if adapter.httpClient.Timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", adapter.httpClient.Timeout)
	}
}

func TestNewTracingAdapter_BaseURLSet(t *testing.T) {
	baseURL := "http://traces-adapter.example.com:9000"
	config := TracingAdapterConfig{
		BaseURL: baseURL,
		Timeout: 30 * time.Second,
	}

	adapter := NewTracingAdapter(config)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}

	if adapter.baseURL != baseURL {
		t.Errorf("Expected baseURL %s, got %s", baseURL, adapter.baseURL)
	}
}

func TestNewTracingAdapter_HTTPClientInitialized(t *testing.T) {
	config := TracingAdapterConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 30 * time.Second,
	}

	adapter := NewTracingAdapter(config)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}

	if adapter.httpClient == nil {
		t.Fatal("Expected non-nil httpClient")
	}
}
