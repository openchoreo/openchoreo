// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkov

import (
	"context"
	"errors"
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
	SetMockScanner(&MockScanner{})
	defer ResetScanner()

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
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) == 0 {
		t.Error("Expected at least one finding for vulnerable manifest")
	}

	for _, finding := range findings {
		if finding.CheckID == "" {
			t.Error("Finding has empty CheckID")
		}
	}

	foundPrivileged := false
	foundRunAsRoot := false
	for _, finding := range findings {
		if finding.CheckID == "CKV_K8S_16" {
			foundPrivileged = true
		}
		if finding.CheckID == "CKV_K8S_40" {
			foundRunAsRoot = true
		}
	}

	if !foundPrivileged {
		t.Error("Expected to find privileged container check (CKV_K8S_16)")
	}
	if !foundRunAsRoot {
		t.Error("Expected to find run as root check (CKV_K8S_40)")
	}
}

func TestRunCheckov_ValidManifest(t *testing.T) {
	SetMockScanner(&MockScanner{})
	defer ResetScanner()

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
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) > 0 {
		t.Logf("Secure manifest has %d findings", len(findings))
	}

	for _, finding := range findings {
		if finding.CheckID == "CKV_K8S_16" {
			t.Error("Should not find privileged container check for secure manifest")
		}
		if finding.CheckID == "CKV_K8S_20" {
			t.Error("Should not find privilege escalation check for secure manifest")
		}
		if finding.CheckID == "CKV_K8S_22" {
			t.Error("Should not find read-only filesystem check for secure manifest")
		}
	}
}

func TestMockScanner_Error(t *testing.T) {
	expectedErr := errors.New("mock error")
	SetMockScanner(&MockScanner{Err: expectedErr})
	defer ResetScanner()

	manifest := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  containers:
  - name: test
    image: nginx
`)

	_, err := RunCheckov(context.Background(), manifest)
	if err == nil {
		t.Error("Expected error from mock scanner")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestMockScanner_CustomFindings(t *testing.T) {
	customFindings := []Finding{
		{
			CheckID:     "CUSTOM_1",
			CheckName:   "Custom Check",
			Severity:    SeverityCritical,
			Category:    "custom",
			Description: "This is a custom finding",
			Remediation: "Fix it",
		},
	}
	SetMockScanner(&MockScanner{Findings: customFindings})
	defer ResetScanner()

	manifest := []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  containers:
  - name: test
    image: nginx
`)

	findings, err := RunCheckov(context.Background(), manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(findings) != 1 {
		t.Errorf("Expected 1 finding, got %d", len(findings))
	}

	if len(findings) > 0 && findings[0].CheckID != "CUSTOM_1" {
		t.Errorf("Expected CUSTOM_1, got %s", findings[0].CheckID)
	}
}
