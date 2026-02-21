// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

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

	fn()

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}
	r.Close()

	return buf.String()
}

func TestPrintClusterComponentTypes_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		if err := PrintClusterComponentTypes(nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster component types found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrintClusterComponentTypes_Empty(t *testing.T) {
	list := &gen.ClusterComponentTypeList{Items: []gen.ClusterComponentType{}}
	out := captureStdout(t, func() {
		if err := PrintClusterComponentTypes(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster component types found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrintClusterComponentTypes_WithItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ClusterComponentTypeSpecWorkloadTypeDeployment
	list := &gen.ClusterComponentTypeList{
		Items: []gen.ClusterComponentType{
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
		},
	}

	out := captureStdout(t, func() {
		if err := PrintClusterComponentTypes(list); err != nil {
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

func TestPrintClusterComponentTypes_NilSpec(t *testing.T) {
	now := time.Now()
	list := &gen.ClusterComponentTypeList{
		Items: []gen.ClusterComponentType{
			{
				Metadata: gen.ObjectMeta{
					Name:              "no-spec-type",
					CreationTimestamp: &now,
				},
				Spec: nil,
			},
		},
	}

	out := captureStdout(t, func() {
		if err := PrintClusterComponentTypes(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "no-spec-type") {
		t.Errorf("expected output to contain 'no-spec-type', got %q", out)
	}
}

func TestPrintClusterTraits_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		if err := PrintClusterTraits(nil); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster traits found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrintClusterTraits_Empty(t *testing.T) {
	list := &gen.ClusterTraitList{Items: []gen.ClusterTrait{}}
	out := captureStdout(t, func() {
		if err := PrintClusterTraits(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster traits found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrintClusterTraits_WithItems(t *testing.T) {
	now := time.Now()
	list := &gen.ClusterTraitList{
		Items: []gen.ClusterTrait{
			{
				Metadata: gen.ObjectMeta{
					Name:              "ingress",
					CreationTimestamp: &now,
				},
			},
			{
				Metadata: gen.ObjectMeta{
					Name: "storage",
				},
			},
		},
	}

	out := captureStdout(t, func() {
		if err := PrintClusterTraits(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// Verify header
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "AGE") {
		t.Errorf("expected table header with NAME, AGE columns, got %q", out)
	}

	// Verify items
	if !strings.Contains(out, "ingress") {
		t.Errorf("expected output to contain 'ingress', got %q", out)
	}
	if !strings.Contains(out, "storage") {
		t.Errorf("expected output to contain 'storage', got %q", out)
	}
}

func TestPrintClusterTraits_NilTimestamp(t *testing.T) {
	list := &gen.ClusterTraitList{
		Items: []gen.ClusterTrait{
			{
				Metadata: gen.ObjectMeta{
					Name:              "no-timestamp",
					CreationTimestamp: nil,
				},
			},
		},
	}

	out := captureStdout(t, func() {
		if err := PrintClusterTraits(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "no-timestamp") {
		t.Errorf("expected output to contain 'no-timestamp', got %q", out)
	}
}
