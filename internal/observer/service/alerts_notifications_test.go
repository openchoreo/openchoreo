// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/labels"
	legacytypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestAlertServiceSendAlertNotification_NoChannels(t *testing.T) {
	svc := &AlertService{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	err := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{AlertName: "test-rule"})
	if err != nil {
		t.Fatalf("expected nil error when no channels configured, got %v", err)
	}
}

func TestAlertServiceSendAlertNotification_PartialFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "email-good-cm",
			Namespace: "default",
			Labels: map[string]string{
				labels.LabelKeyNotificationChannelName: "email-good",
			},
		},
		Data: map[string]string{
			"type":      "email",
			"smtp.host": "smtp.example.com",
			"to":        "[alerts@example.com]",
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "email-good-secret",
			Namespace: "default",
			Labels: map[string]string{
				labels.LabelKeyNotificationChannelName: "email-good",
			},
		},
		Data: map[string][]byte{},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, secret).Build()

	svc := &AlertService{
		k8sClient: k8sClient,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	err := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{
		AlertName:            "test-rule",
		NotificationChannels: []string{"email-good", "missing-channel"},
	})
	if err == nil {
		t.Fatal("expected aggregated error for missing channel")
	}

	if !strings.Contains(err.Error(), "missing-channel") {
		t.Fatalf("expected error to mention missing-channel, got %v", err)
	}
}
