// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

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
	if !strings.Contains(out, "No cluster traits found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrint_Empty(t *testing.T) {
	list := &gen.ClusterTraitList{Items: []gen.ClusterTrait{}}
	out := captureStdout(t, func() {
		if err := printList(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "No cluster traits found") {
		t.Errorf("expected empty message, got %q", out)
	}
}

func TestPrint_WithItems(t *testing.T) {
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
		if err := printList(list); err != nil {
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

func TestPrint_NilTimestamp(t *testing.T) {
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
		if err := printList(list); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "no-timestamp") {
		t.Errorf("expected output to contain 'no-timestamp', got %q", out)
	}
}
