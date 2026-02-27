// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// captureStdout captures stdout output from a function call.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}

	return buf.String()
}

func TestPrint_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		if err := printList(nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster component types found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		if err := printList([]gen.ClusterComponentType{}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster component types found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ClusterComponentTypeSpecWorkloadTypeDeployment
	items := []gen.ClusterComponentType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "web-app",
				CreationTimestamp: &now,
			},
			Spec: &gen.ClusterComponentTypeSpec{
				WorkloadType: workloadType,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "batch-job",
			},
		},
	}

	out := captureStdout(t, func() {
		if err := printList(items); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// Verify header
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "WORKLOAD TYPE") || !strings.Contains(out, "AGE") {
		t.Errorf("expected table header with NAME, WORKLOAD TYPE, AGE columns, got %q", out)
	}

	// Verify items
	if !strings.Contains(out, "web-app") {
		t.Errorf("expected output to contain 'web-app', got %q", out)
	}
	if !strings.Contains(out, "deployment") {
		t.Errorf("expected output to contain 'deployment', got %q", out)
	}
	if !strings.Contains(out, "batch-job") {
		t.Errorf("expected output to contain 'batch-job', got %q", out)
	}
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterComponentType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-spec-type",
				CreationTimestamp: &now,
			},
			Spec: nil,
		},
	}

	out := captureStdout(t, func() {
		if err := printList(items); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "no-spec-type") {
		t.Errorf("expected output to contain 'no-spec-type', got %q", out)
	}
}
