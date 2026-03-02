// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"
	"time"
)

func TestNewTracesBackend_DefaultTimeout(t *testing.T) {
	config := TracesBackendConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 0, // Should use default
	}

	backend := NewTracesBackend(config)
	if backend == nil {
		t.Fatal("Expected non-nil backend")
	}

	if backend.httpClient.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", backend.httpClient.Timeout)
	}
}

func TestNewTracesBackend_CustomTimeout(t *testing.T) {
	config := TracesBackendConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 60 * time.Second,
	}

	backend := NewTracesBackend(config)
	if backend == nil {
		t.Fatal("Expected non-nil backend")
	}

	if backend.httpClient.Timeout != 60*time.Second {
		t.Errorf("Expected timeout 60s, got %v", backend.httpClient.Timeout)
	}
}

func TestNewTracesBackend_BaseURLSet(t *testing.T) {
	baseURL := "http://traces-backend.example.com:9000"
	config := TracesBackendConfig{
		BaseURL: baseURL,
		Timeout: 30 * time.Second,
	}

	backend := NewTracesBackend(config)
	if backend == nil {
		t.Fatal("Expected non-nil backend")
	}

	if backend.baseURL != baseURL {
		t.Errorf("Expected baseURL %s, got %s", baseURL, backend.baseURL)
	}
}

func TestNewTracesBackend_HTTPClientInitialized(t *testing.T) {
	config := TracesBackendConfig{
		BaseURL: "http://localhost:8080",
		Timeout: 30 * time.Second,
	}

	backend := NewTracesBackend(config)
	if backend == nil {
		t.Fatal("Expected non-nil backend")
	}

	if backend.httpClient == nil {
		t.Fatal("Expected non-nil httpClient")
	}
}
