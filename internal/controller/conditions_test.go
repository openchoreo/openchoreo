// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"
	"testing"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

func TestNeedConditionUpdate(t *testing.T) {
	tests := []struct {
		name              string
		currentConditions []metav1.Condition
		updatedConditions []metav1.Condition
		want              bool
	}{
		{
			name:              "Both conditions empty -> No update needed",
			currentConditions: []metav1.Condition{},
			updatedConditions: []metav1.Condition{},
			want:              false,
		},
		{
			name:              "Different lengths -> Update needed (current is empty, updated has 1)",
			currentConditions: []metav1.Condition{},
			updatedConditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
			want: true,
		},
		{
			name: "Different lengths -> Update needed (current has 1, updated is empty)",
			currentConditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
			updatedConditions: []metav1.Condition{},
			want:              true,
		},
		{
			name: "Same conditions -> No update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is okay",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is okay",
					ObservedGeneration: 1,
				},
			},
			want: false,
		},
		{
			name: "Status changed -> Update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "False",
					Reason:             "NotReady",
					Message:            "Some issue",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is okay now",
					ObservedGeneration: 1,
				},
			},
			want: true,
		},
		{
			name: "Reason changed -> Update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "OldReason",
					Message:            "No updates",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "NewReason",
					Message:            "No updates",
					ObservedGeneration: 1,
				},
			},
			want: true,
		},
		{
			name: "Message changed -> Update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Old message",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "New message",
					ObservedGeneration: 1,
				},
			},
			want: true,
		},
		{
			name: "ObservedGeneration changed -> Update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "No changes",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "No changes",
					ObservedGeneration: 2,
				},
			},
			want: true,
		},
		{
			name: "New condition added -> Update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is fine",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is fine",
					ObservedGeneration: 1,
				},
				{
					Type:               "Healthy",
					Status:             "True",
					Reason:             "DiagnosticsPassed",
					Message:            "Diagnostics look good",
					ObservedGeneration: 1,
				},
			},
			want: true,
		},
		{
			name: "Condition removed -> Update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is fine",
					ObservedGeneration: 1,
				},
				{
					Type:               "Healthy",
					Status:             "True",
					Reason:             "DiagnosticsPassed",
					Message:            "Diagnostics look good",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is fine",
					ObservedGeneration: 1,
				},
			},
			want: true,
		},
		{
			name: "Unchanged multiple conditions -> No update needed",
			currentConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is fine",
					ObservedGeneration: 1,
				},
				{
					Type:               "Healthy",
					Status:             "True",
					Reason:             "DiagnosticsPassed",
					Message:            "Diagnostics look good",
					ObservedGeneration: 1,
				},
			},
			updatedConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             "True",
					Reason:             "AllGood",
					Message:            "Everything is fine",
					ObservedGeneration: 1,
				},
				{
					Type:               "Healthy",
					Status:             "True",
					Reason:             "DiagnosticsPassed",
					Message:            "Diagnostics look good",
					ObservedGeneration: 1,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedConditionUpdate(tt.currentConditions, tt.updatedConditions); got != tt.want {
				t.Errorf("NeedConditionUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Every rendering reconciler copies a render error into a condition message, and each
// layer that builds one is expected to bound the fragments it quotes. This is the backstop
// for the layer that forgets: the API server caps a condition message at 32768 bytes and
// rejects invalid UTF-8, and a rejected status write replaces the paced requeue with a hot
// backoff. Bounding here means no single missed wrap can cause that.
func TestNewConditionBoundsTheMessage(t *testing.T) {
	huge := strings.Repeat("x", 100_000)

	cond := NewCondition("Ready", metav1.ConditionFalse, "RenderingFailed", huge, 1)

	if len(cond.Message) > maxConditionMessageLen {
		t.Fatalf("message is %d bytes, want at most %d", len(cond.Message), maxConditionMessageLen)
	}
	if !strings.HasSuffix(cond.Message, template.TruncationMarker) {
		t.Fatalf("a cut message should be marked truncated, got: %.40s...", cond.Message)
	}
	if !strings.HasPrefix(cond.Message, "xxxx") {
		t.Fatalf("the surviving prefix should still be the original message, got: %.40s", cond.Message)
	}
}

// A message that fits is the common case and must be untouched, byte for byte: the paced
// requeue depends on the same inputs producing the same condition on every reconcile.
func TestNewConditionKeepsMessagesThatFit(t *testing.T) {
	msg := "failed to render resources: no such field: spec.nope"

	cond := NewCondition("Ready", metav1.ConditionFalse, "RenderingFailed", msg, 1)

	if cond.Message != msg {
		t.Fatalf("message was altered: got %q, want %q", cond.Message, msg)
	}
}

// Cutting mid-rune produces invalid UTF-8, which the API server rejects just as firmly as
// an oversized message - so the bound has to land on a rune boundary regardless of where
// the multi-byte characters fall.
func TestNewConditionBoundKeepsValidUTF8(t *testing.T) {
	for pad := 0; pad < 4; pad++ {
		msg := strings.Repeat("a", pad) + strings.Repeat("日", maxConditionMessageLen)

		cond := NewCondition("Ready", metav1.ConditionFalse, "RenderingFailed", msg, 1)

		if !utf8.ValidString(cond.Message) {
			t.Fatalf("pad %d produced invalid UTF-8", pad)
		}
		if len(cond.Message) > maxConditionMessageLen {
			t.Fatalf("pad %d: message is %d bytes, want at most %d",
				pad, len(cond.Message), maxConditionMessageLen)
		}
	}
}

// The bound applies through the Mark* helpers too, which is how reconcilers actually
// record a render failure.
func TestMarkFalseConditionBoundsTheMessage(t *testing.T) {
	rb := &openchoreov1alpha1.ReleaseBinding{}

	MarkFalseCondition(rb, "ReleaseSynced", "RenderingFailed", strings.Repeat("y", 100_000))

	conditions := rb.GetConditions()
	if len(conditions) != 1 {
		t.Fatalf("expected one condition, got %d", len(conditions))
	}
	if len(conditions[0].Message) > maxConditionMessageLen {
		t.Fatalf("message is %d bytes, want at most %d", len(conditions[0].Message), maxConditionMessageLen)
	}
}
