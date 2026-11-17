// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/observer/notifications"
	legacytypes "github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestAlertServiceSendAlertNotification_NoChannels(t *testing.T) {
	svc := &AlertService{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	results := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{AlertName: "test-rule"})
	if len(results) != 0 {
		t.Fatalf("expected no results when no channels configured, got %v", results)
	}
}

func TestAlertServiceSendAlertNotification_PartialFailure(t *testing.T) {
	var good webhookRecorder
	goodCM, goodSecret := webhookChannelObjects("default", "webhook-good", good.start(t), "Bearer TOKEN")
	svc := newNotificationTestService(t, goodCM, goodSecret)

	results := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{
		AlertName:            "test-rule",
		Namespace:            "default",
		NotificationChannels: []string{"webhook-good", "missing-channel"},
	})
	if len(results) != 2 {
		t.Fatalf("expected a result per channel, got %v", results)
	}
	if !results[0].Sent || results[0].Channel != "webhook-good" || results[0].ChannelType != "webhook" {
		t.Errorf("expected webhook-good to be sent, got %+v", results[0])
	}
	if results[1].Sent || results[1].Channel != "missing-channel" || results[1].Reason != notifications.ReasonChannelConfig {
		t.Errorf("expected missing-channel to fail with reason channel_config, got %+v", results[1])
	}
	if got := good.received(); len(got) != 1 {
		t.Errorf("working channel received %v, want one request", got)
	}
}

// webhookChannelObjects returns the ConfigMap and Secret the controller applies to the observability plane
// for a webhook channel that sends the given Authorization header to url.
func webhookChannelObjects(namespace, name, url, authorization string) (*corev1.ConfigMap, *corev1.Secret) {
	channelLabels := map[string]string{labels.LabelKeyNotificationChannelName: name}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: channelLabels},
		Data: map[string]string{
			"type":            "webhook",
			"webhook.url":     url,
			"webhook.headers": "Authorization",
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: channelLabels},
		Data:       map[string][]byte{"webhook.header.Authorization": []byte(authorization)},
	}
	return configMap, secret
}

// webhookRecorder records the Authorization header of every request it receives.
type webhookRecorder struct {
	mu             sync.Mutex
	authorizations []string
}

func (r *webhookRecorder) start(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.authorizations = append(r.authorizations, req.Header.Get("Authorization"))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func (r *webhookRecorder) received() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.authorizations...)
}

func newNotificationTestService(t *testing.T, objs ...client.Object) *AlertService {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed adding corev1 scheme: %v", err)
	}
	return &AlertService{
		k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestAlertServiceSendAlertNotification_ResolvesChannelInAlertNamespace(t *testing.T) {
	var own, other webhookRecorder
	// The other namespace sorts first, so a lookup across namespaces would pick its channel.
	otherCM, otherSecret := webhookChannelObjects("a-other", "oncall", other.start(t), "Bearer OTHER-TOKEN")
	ownCM, ownSecret := webhookChannelObjects("tenant", "oncall", own.start(t), "Bearer OWN-TOKEN")
	svc := newNotificationTestService(t, otherCM, otherSecret, ownCM, ownSecret)

	results := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{
		AlertName:            "test-rule",
		Namespace:            "tenant",
		NotificationChannels: []string{"oncall"},
	})
	if len(results) != 1 || !results[0].Sent {
		t.Fatalf("sendAlertNotification() results = %+v, want one sent result", results)
	}

	if got := own.received(); len(got) != 1 || got[0] != "Bearer OWN-TOKEN" {
		t.Errorf("channel in the alert's namespace received %v, want one request with Bearer OWN-TOKEN", got)
	}
	if got := other.received(); len(got) != 0 {
		t.Errorf("same-named channel in another namespace received %v, want no requests", got)
	}
}

func TestAlertServiceSendAlertNotification_ChannelOnlyInOtherNamespace(t *testing.T) {
	var other webhookRecorder
	otherCM, otherSecret := webhookChannelObjects("other", "oncall", other.start(t), "Bearer OTHER-TOKEN")
	svc := newNotificationTestService(t, otherCM, otherSecret)

	results := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{
		AlertName:            "test-rule",
		Namespace:            "tenant",
		NotificationChannels: []string{"oncall"},
	})
	if len(results) != 1 || results[0].Sent {
		t.Fatal("expected a failed result for a channel that is not in the alert's namespace")
	}
	if got := other.received(); len(got) != 0 {
		t.Errorf("channel in another namespace received %v, want no requests", got)
	}
}

func TestAlertServiceSendAlertNotification_DoesNotMixNamespaces(t *testing.T) {
	var first webhookRecorder
	// The channel in the first namespace has no Secret on the plane yet, so a lookup across namespaces would
	// pair its ConfigMap with the Secret of the same-named channel in another namespace.
	firstCM, _ := webhookChannelObjects("a-first", "oncall", first.start(t), "")
	_, otherSecret := webhookChannelObjects("other", "oncall", "http://other.invalid", "Bearer OTHER-TOKEN")
	svc := newNotificationTestService(t, firstCM, otherSecret)

	results := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{
		AlertName:            "test-rule",
		Namespace:            "a-first",
		NotificationChannels: []string{"oncall"},
	})
	if len(results) != 1 || results[0].Sent {
		t.Fatal("expected a failed result for a channel whose Secret is missing")
	}
	if got := first.received(); len(got) != 0 {
		t.Errorf("channel without a Secret received %v, want no requests", got)
	}
}

func TestAlertServiceSendAlertNotification_WebhookURLFromSecret(t *testing.T) {
	recorder := &webhookRecorder{}
	configMap, secret := webhookChannelObjects("tenant", "oncall", recorder.start(t), "Bearer TOKEN")
	// A URL that comes from a secret reference is in the Secret, not in the ConfigMap.
	secret.Data["webhook.url"] = []byte(configMap.Data["webhook.url"] + "\n")
	delete(configMap.Data, "webhook.url")
	svc := newNotificationTestService(t, configMap, secret)

	results := svc.sendAlertNotification(context.Background(), &legacytypes.AlertDetails{
		AlertName:            "test-rule",
		Namespace:            "tenant",
		NotificationChannels: []string{"oncall"},
	})
	if len(results) != 1 || !results[0].Sent {
		t.Fatalf("sendAlertNotification() results = %+v, want one sent result", results)
	}
	if got := recorder.received(); len(got) != 1 || got[0] != "Bearer TOKEN" {
		t.Errorf("webhook received %v, want one request with the channel's Authorization header", got)
	}
}

func TestAlertServiceGetNotificationChannelConfig(t *testing.T) {
	labeledCM, labeledSecret := webhookChannelObjects("tenant", "oncall", "http://example.invalid", "Bearer TOKEN")
	unlabeledCM, unlabeledSecret := webhookChannelObjects("tenant", "unlabeled", "http://example.invalid", "Bearer TOKEN")
	unlabeledCM.Labels = nil
	unlabeledSecret.Labels = nil
	svc := newNotificationTestService(t, labeledCM, labeledSecret, unlabeledCM, unlabeledSecret)

	tests := []struct {
		name      string
		namespace string
		channel   string
		wantErr   string
	}{
		{name: "channel in namespace", namespace: "tenant", channel: "oncall"},
		{name: "empty namespace", namespace: "", channel: "oncall", wantErr: "no namespace"},
		{name: "resources without the channel label", namespace: "tenant", channel: "unlabeled", wantErr: "is not labeled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := svc.getNotificationChannelConfig(context.Background(), tt.namespace, tt.channel)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("getNotificationChannelConfig() error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("getNotificationChannelConfig() error = %v", err)
			}
			if got := cfg.Webhook.Headers["Authorization"]; got != "Bearer TOKEN" {
				t.Errorf("Authorization header = %q, want %q", got, "Bearer TOKEN")
			}
		})
	}
}
