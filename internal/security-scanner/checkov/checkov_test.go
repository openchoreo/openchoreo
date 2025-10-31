// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkov

import (
	"context"
	"testing"
)

func TestMapSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected Severity
	}{
		{"CRITICAL", SeverityCritical},
		{"critical", SeverityCritical},
		{"HIGH", SeverityHigh},
		{"high", SeverityHigh},
		{"MEDIUM", SeverityMedium},
		{"medium", SeverityMedium},
		{"LOW", SeverityLow},
		{"low", SeverityLow},
		{"INFO", SeverityInfo},
		{"info", SeverityInfo},
		{"INFORMATIONAL", SeverityInfo},
		{"unknown", SeverityUnknown},
		{"", SeverityMedium},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapSeverity(tt.input)
			if result != tt.expected {
				t.Errorf("mapSeverity(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCategorizeCheck(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"checkov.kubernetes.SecurityCheck", "security"},
		{"checkov.kubernetes.NetworkCheck", "network"},
		{"NetworkPolicyCheck", "networkpolicy"},
		{"SecurityCheck", "security"},
		{"", "security"},
		{"checkov", "checkov"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := categorizeCheck(tt.input)
			if result != tt.expected {
				t.Errorf("categorizeCheck(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRunCheckov(t *testing.T) {
	manifest := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: vulnerable-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: vulnerable
  template:
    metadata:
      labels:
        app: vulnerable
    spec:
      containers:
      - name: app
        image: nginx:latest
        securityContext:
          privileged: true
          runAsUser: 0
        resources: {}
`)

	findings, err := RunCheckov(context.Background(), manifest)
	if err != nil {
		t.Skipf("Skipping test because checkov is not installed: %v", err)
		return
	}

	if len(findings) == 0 {
		t.Error("Expected at least one finding for vulnerable manifest")
	}

	for _, finding := range findings {
		if finding.CheckID == "" {
			t.Error("Finding has empty CheckID")
		}
	}
}

func TestRunCheckov_ValidManifest(t *testing.T) {
	manifest := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: secure-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: secure
  template:
    metadata:
      labels:
        app: secure
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 2000
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: app
        image: nginx:1.21@sha256:abc123
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        resources:
          limits:
            cpu: 100m
            memory: 128Mi
          requests:
            cpu: 100m
            memory: 128Mi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
`)

	findings, err := RunCheckov(context.Background(), manifest)
	if err != nil {
		t.Skipf("Skipping test because checkov is not installed: %v", err)
		return
	}

	if len(findings) > 5 {
		t.Logf("Secure manifest still has %d findings (this is expected as checkov has many checks)", len(findings))
	}
}
