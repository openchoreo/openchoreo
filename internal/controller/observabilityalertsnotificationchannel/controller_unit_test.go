// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(openchoreo): %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(corev1): %v", err)
	}
	return s
}

func strPtr(s string) *string { return &s }

func newReconcilerWithObjects(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	s := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return &Reconciler{
		Client: c,
		Scheme: s,
	}
}

func newEmailChannel(name, env string) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "ObservabilityAlertsNotificationChannel",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
			Environment: env,
			Type:        openchoreov1alpha1.NotificationChannelTypeEmail,
			EmailConfig: &openchoreov1alpha1.EmailConfig{
				From: "sender@example.com",
				To:   []string{"a@example.com", "b@example.com"},
				SMTP: openchoreov1alpha1.SMTPConfig{
					Host: "smtp.example.com",
					Port: 587,
				},
			},
		},
	}
}

func newWebhookChannel(name, env string) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel { //nolint:unparam // env is parameterized for symmetry with newEmailChannel and future tests
	return &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "ObservabilityAlertsNotificationChannel",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
			Environment: env,
			Type:        openchoreov1alpha1.NotificationChannelTypeWebhook,
			WebhookConfig: &openchoreov1alpha1.WebhookConfig{
				URL: "https://hooks.example.com/services/abc",
			},
		},
	}
}

// ---------------------------------------------------------------------------
// createConfigMap
// ---------------------------------------------------------------------------

func TestCreateConfigMap_EmailFull(t *testing.T) {
	const channelName = "email-full"
	r := newReconcilerWithObjects(t)
	ch := newEmailChannel(channelName, "development")
	ch.Spec.IsEnvDefault = true
	ch.Spec.EmailConfig.Template = &openchoreov1alpha1.EmailTemplate{
		Subject: "[${alert.severity}] - ${alert.name}",
		Body:    "Alert ${alert.name} fired",
	}
	ch.Spec.EmailConfig.SMTP.TLS = &openchoreov1alpha1.SMTPTLSConfig{InsecureSkipVerify: true}

	cm := r.createConfigMap(ch)

	if cm.Name != channelName {
		t.Errorf("Name: got %q, want %q", cm.Name, channelName)
	}
	if cm.Namespace != "default" {
		t.Errorf("Namespace: got %q, want %q", cm.Namespace, "default")
	}
	if got, want := cm.Labels["app.kubernetes.io/managed-by"], "observabilityalertsnotificationchannel-controller"; got != want {
		t.Errorf("managed-by label: got %q, want %q", got, want)
	}
	if got, want := cm.Labels["app.kubernetes.io/name"], channelName; got != want {
		t.Errorf("name label: got %q, want %q", got, want)
	}
	if got, want := cm.Labels[labels.LabelKeyNotificationChannelName], channelName; got != want {
		t.Errorf("notification-channel label: got %q, want %q", got, want)
	}

	checks := []struct{ key, want string }{
		{"type", "email"},
		{"isEnvDefault", "true"},
		{"from", "sender@example.com"},
		{"smtp.host", "smtp.example.com"},
		{"smtp.port", "587"},
		{"smtp.tls.insecureSkipVerify", "true"},
		{"template.subject", "[${alert.severity}] - ${alert.name}"},
		{"template.body", "Alert ${alert.name} fired"},
	}
	for _, c := range checks {
		if got := cm.Data[c.key]; got != c.want {
			t.Errorf("Data[%q]: got %q, want %q", c.key, got, c.want)
		}
	}
	if !strings.Contains(cm.Data["to"], "a@example.com") || !strings.Contains(cm.Data["to"], "b@example.com") {
		t.Errorf("Data[to] missing recipients: %q", cm.Data["to"])
	}
}

func TestCreateConfigMap_EmailWithoutTemplateOrTLS(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newEmailChannel("email-min", "dev")

	cm := r.createConfigMap(ch)

	if _, ok := cm.Data["template.subject"]; ok {
		t.Error("template.subject should be absent when Template is nil")
	}
	if _, ok := cm.Data["template.body"]; ok {
		t.Error("template.body should be absent when Template is nil")
	}
	if _, ok := cm.Data["smtp.tls.insecureSkipVerify"]; ok {
		t.Error("smtp.tls.insecureSkipVerify should be absent when TLS is nil")
	}
	if cm.Data["isEnvDefault"] != "false" {
		t.Errorf("isEnvDefault: got %q, want %q", cm.Data["isEnvDefault"], "false")
	}
}

func TestCreateConfigMap_WebhookInlineHeaders(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newWebhookChannel("wh-inline", "dev")
	ch.Spec.WebhookConfig.PayloadTemplate = `{"text": "${alertName}"}`
	ch.Spec.WebhookConfig.Headers = map[string]openchoreov1alpha1.WebhookHeaderValue{
		"X-Source": {Value: strPtr("openchoreo")},
		"X-Tenant": {Value: strPtr("acme")},
	}

	cm := r.createConfigMap(ch)

	if cm.Data["type"] != "webhook" {
		t.Errorf("type: got %q, want %q", cm.Data["type"], "webhook")
	}
	if cm.Data["webhook.url"] != "https://hooks.example.com/services/abc" {
		t.Errorf("webhook.url: got %q", cm.Data["webhook.url"])
	}
	if cm.Data["webhook.payloadTemplate"] != `{"text": "${alertName}"}` {
		t.Errorf("webhook.payloadTemplate: got %q", cm.Data["webhook.payloadTemplate"])
	}
	if cm.Data["webhook.header.X-Source"] != "openchoreo" {
		t.Errorf("inline header X-Source: got %q", cm.Data["webhook.header.X-Source"])
	}
	if cm.Data["webhook.header.X-Tenant"] != "acme" {
		t.Errorf("inline header X-Tenant: got %q", cm.Data["webhook.header.X-Tenant"])
	}
	// webhook.headers is a comma-joined list of header names (any order)
	got := cm.Data["webhook.headers"]
	if !(strings.Contains(got, "X-Source") && strings.Contains(got, "X-Tenant")) {
		t.Errorf("webhook.headers should contain both header names; got %q", got)
	}
}

func TestCreateConfigMap_WebhookSecretRefHeaders(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newWebhookChannel("wh-secret", "dev")
	ch.Spec.WebhookConfig.Headers = map[string]openchoreov1alpha1.WebhookHeaderValue{
		"Authorization": {
			ValueFrom: &openchoreov1alpha1.SecretValueFrom{
				SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "wh-auth", Key: "token"},
			},
		},
	}

	cm := r.createConfigMap(ch)

	if _, ok := cm.Data["webhook.header.Authorization"]; ok {
		t.Error("secret-ref header value must not appear in ConfigMap data")
	}
	if cm.Data["webhook.headers"] != "Authorization" {
		t.Errorf("webhook.headers: got %q, want %q", cm.Data["webhook.headers"], "Authorization")
	}
}

func TestCreateConfigMap_WebhookNoHeadersOrTemplate(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newWebhookChannel("wh-bare", "dev")

	cm := r.createConfigMap(ch)

	if cm.Data["webhook.url"] != "https://hooks.example.com/services/abc" {
		t.Errorf("webhook.url: got %q", cm.Data["webhook.url"])
	}
	if _, ok := cm.Data["webhook.headers"]; ok {
		t.Error("webhook.headers should be absent when no headers are configured")
	}
	if _, ok := cm.Data["webhook.payloadTemplate"]; ok {
		t.Error("webhook.payloadTemplate should be absent when not configured")
	}
}

// ---------------------------------------------------------------------------
// createSecret
// ---------------------------------------------------------------------------

func TestCreateSecret_EmailNoAuth(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newEmailChannel("no-auth", "dev")

	sec, err := r.createSecret(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sec.Type != corev1.SecretTypeOpaque {
		t.Errorf("Secret.Type: got %q, want %q", sec.Type, corev1.SecretTypeOpaque)
	}
	if got, want := sec.Labels[labels.LabelKeyNotificationChannelName], "no-auth"; got != want {
		t.Errorf("label: got %q, want %q", got, want)
	}
	if len(sec.Data) != 0 {
		t.Errorf("Data should be empty when no auth/headers; got %v", sec.Data)
	}
}

func TestCreateSecret_EmailAuthResolved(t *testing.T) {
	srcSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "smtp-creds", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("alice"),
			// Include trailing newline to verify resolveSecretKeyRef trims whitespace.
			"password": []byte("s3cret\n"),
		},
	}
	r := newReconcilerWithObjects(t, srcSecret)

	ch := newEmailChannel("with-auth", "dev")
	ch.Spec.EmailConfig.SMTP.Auth = &openchoreov1alpha1.SMTPAuth{
		Username: &openchoreov1alpha1.SecretValueFrom{
			SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "smtp-creds", Key: "username"},
		},
		Password: &openchoreov1alpha1.SecretValueFrom{
			SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "smtp-creds", Key: "password"},
		},
	}

	sec, err := r.createSecret(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(sec.Data["smtp.auth.username"]), "alice"; got != want {
		t.Errorf("smtp.auth.username: got %q, want %q", got, want)
	}
	if got, want := string(sec.Data["smtp.auth.password"]), "s3cret"; got != want {
		t.Errorf("smtp.auth.password: got %q, want %q (whitespace must be trimmed)", got, want)
	}
}

func TestCreateSecret_EmailAuthSecretMissing(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newEmailChannel("missing-secret", "dev")
	ch.Spec.EmailConfig.SMTP.Auth = &openchoreov1alpha1.SMTPAuth{
		Username: &openchoreov1alpha1.SecretValueFrom{
			SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "nope", Key: "username"},
		},
	}

	_, err := r.createSecret(context.Background(), ch)
	if err == nil {
		t.Fatal("expected error when SMTP username secret is missing, got nil")
	}
	if !strings.Contains(err.Error(), "SMTP username") {
		t.Errorf("error should mention SMTP username; got %q", err.Error())
	}
}

func TestCreateSecret_EmailAuthKeyMissing(t *testing.T) {
	srcSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "smtp-creds", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"username": []byte("alice")},
	}
	r := newReconcilerWithObjects(t, srcSecret)
	ch := newEmailChannel("missing-key", "dev")
	ch.Spec.EmailConfig.SMTP.Auth = &openchoreov1alpha1.SMTPAuth{
		Password: &openchoreov1alpha1.SecretValueFrom{
			SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "smtp-creds", Key: "password"},
		},
	}

	_, err := r.createSecret(context.Background(), ch)
	if err == nil {
		t.Fatal("expected error when key is missing in secret, got nil")
	}
	if !strings.Contains(err.Error(), "SMTP password") {
		t.Errorf("error should mention SMTP password; got %q", err.Error())
	}
}

func TestCreateSecret_WebhookHeaderResolved(t *testing.T) {
	srcSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "wh-auth", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"token": []byte("Bearer xyz")},
	}
	r := newReconcilerWithObjects(t, srcSecret)
	ch := newWebhookChannel("wh-header-secret", "dev")
	ch.Spec.WebhookConfig.Headers = map[string]openchoreov1alpha1.WebhookHeaderValue{
		"Authorization": {
			ValueFrom: &openchoreov1alpha1.SecretValueFrom{
				SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "wh-auth", Key: "token"},
			},
		},
		"X-Inline": {Value: strPtr("inline-value")},
	}

	sec, err := r.createSecret(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(sec.Data["webhook.header.Authorization"]), "Bearer xyz"; got != want {
		t.Errorf("webhook.header.Authorization: got %q, want %q", got, want)
	}
	// Inline header values stay in ConfigMap; not duplicated into Secret.
	if _, ok := sec.Data["webhook.header.X-Inline"]; ok {
		t.Error("inline header value must not be copied into Secret")
	}
}

func TestCreateSecret_WebhookHeaderSecretMissing(t *testing.T) {
	r := newReconcilerWithObjects(t)
	ch := newWebhookChannel("wh-missing", "dev")
	ch.Spec.WebhookConfig.Headers = map[string]openchoreov1alpha1.WebhookHeaderValue{
		"Authorization": {
			ValueFrom: &openchoreov1alpha1.SecretValueFrom{
				SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "nope", Key: "token"},
			},
		},
	}

	_, err := r.createSecret(context.Background(), ch)
	if err == nil {
		t.Fatal("expected error when webhook header secret is missing, got nil")
	}
	if !strings.Contains(err.Error(), "webhook header") {
		t.Errorf("error should mention webhook header; got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// resolveSecretKeyRef
// ---------------------------------------------------------------------------

func TestResolveSecretKeyRef(t *testing.T) {
	srcSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"plain":   []byte("plain-value"),
			"padded":  []byte("  spaced  "),
			"newline": []byte("token\n"),
		},
	}
	r := newReconcilerWithObjects(t, srcSecret)
	ctx := context.Background()

	t.Run("plain value", func(t *testing.T) {
		v, err := r.resolveSecretKeyRef(ctx, "default", &openchoreov1alpha1.SecretKeyRef{Name: "creds", Key: "plain"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "plain-value" {
			t.Errorf("got %q, want %q", v, "plain-value")
		}
	})

	t.Run("trims trailing newline", func(t *testing.T) {
		v, err := r.resolveSecretKeyRef(ctx, "default", &openchoreov1alpha1.SecretKeyRef{Name: "creds", Key: "newline"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "token" {
			t.Errorf("got %q, want %q", v, "token")
		}
	})

	t.Run("trims surrounding spaces", func(t *testing.T) {
		v, err := r.resolveSecretKeyRef(ctx, "default", &openchoreov1alpha1.SecretKeyRef{Name: "creds", Key: "padded"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "spaced" {
			t.Errorf("got %q, want %q", v, "spaced")
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		_, err := r.resolveSecretKeyRef(ctx, "default", &openchoreov1alpha1.SecretKeyRef{Name: "nope", Key: "plain"})
		if err == nil {
			t.Fatal("expected error for missing secret, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get secret") {
			t.Errorf("error should describe secret get failure; got %q", err.Error())
		}
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := r.resolveSecretKeyRef(ctx, "default", &openchoreov1alpha1.SecretKeyRef{Name: "creds", Key: "missing"})
		if err == nil {
			t.Fatal("expected error for missing key, got nil")
		}
		if !strings.Contains(err.Error(), "key missing not found") {
			t.Errorf("error should describe missing key; got %q", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// ensureDefaultChannel
// ---------------------------------------------------------------------------

func TestEnsureDefaultChannel_AlreadyDefault(t *testing.T) {
	ch := newEmailChannel("ch1", "dev")
	ch.Spec.IsEnvDefault = true
	r := newReconcilerWithObjects(t, ch)

	changed, err := r.ensureDefaultChannel(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected no change when channel is already default")
	}
}

func TestEnsureDefaultChannel_FirstChannelBecomesDefault(t *testing.T) {
	ch := newEmailChannel("only", "dev")
	r := newReconcilerWithObjects(t, ch)

	changed, err := r.ensureDefaultChannel(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected first channel in env to be marked as default")
	}

	// Verify the patch persisted in the fake client.
	got := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: "only", Namespace: "default"}, got); err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if !got.Spec.IsEnvDefault {
		t.Error("expected IsEnvDefault=true to be persisted")
	}
}

func TestEnsureDefaultChannel_ExistingDefaultLeavesNewChannelUnchanged(t *testing.T) {
	existing := newEmailChannel("ch1", "dev")
	existing.Spec.IsEnvDefault = true
	// new channel for the same environment; should remain non-default.
	newCh := newEmailChannel("ch2", "dev")
	r := newReconcilerWithObjects(t, existing, newCh)

	changed, err := r.ensureDefaultChannel(context.Background(), newCh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected no change when an env default already exists")
	}
	if newCh.Spec.IsEnvDefault {
		t.Error("new channel must not be marked default when one already exists")
	}
}

func TestEnsureDefaultChannel_DifferentEnvironmentDoesNotInterfere(t *testing.T) {
	other := newEmailChannel("ch-prod", "production")
	other.Spec.IsEnvDefault = true
	target := newEmailChannel("ch-dev", "dev")
	r := newReconcilerWithObjects(t, other, target)

	changed, err := r.ensureDefaultChannel(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected target to be marked default because no default exists in its env")
	}
	if !target.Spec.IsEnvDefault {
		t.Error("expected target.Spec.IsEnvDefault=true")
	}
}

func TestEnsureDefaultChannel_SkipsChannelsBeingDeleted(t *testing.T) {
	// A channel that's mid-deletion in the same env must not block the new one
	// from becoming the default.
	now := metav1.Now()
	deleting := newEmailChannel("ch-deleting", "dev")
	deleting.DeletionTimestamp = &now
	// Channels with a deletion timestamp need a finalizer for the fake client
	// to accept the object (it otherwise rejects creation with a deletion ts).
	deleting.Finalizers = []string{NotificationChannelCleanupFinalizer}
	deleting.Spec.IsEnvDefault = true

	target := newEmailChannel("ch-new", "dev")
	r := newReconcilerWithObjects(t, deleting, target)

	changed, err := r.ensureDefaultChannel(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected target to become default because the existing default is being deleted")
	}
}

// ---------------------------------------------------------------------------
// ensureFinalizer
// ---------------------------------------------------------------------------

func TestEnsureFinalizer_AddsFinalizer(t *testing.T) {
	ch := newEmailChannel("ch-fin", "dev")
	r := newReconcilerWithObjects(t, ch)

	added, err := r.ensureFinalizer(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !added {
		t.Fatal("expected ensureFinalizer to add the finalizer")
	}
	if !controllerutil.ContainsFinalizer(ch, NotificationChannelCleanupFinalizer) {
		t.Error("finalizer should be present on the channel after ensureFinalizer")
	}

	// Calling it again should be a no-op.
	added2, err := r.ensureFinalizer(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added2 {
		t.Error("ensureFinalizer should be idempotent")
	}
}

func TestEnsureFinalizer_SkipsWhenDeleting(t *testing.T) {
	now := metav1.Now()
	ch := newEmailChannel("ch-del", "dev")
	ch.DeletionTimestamp = &now
	ch.Finalizers = []string{NotificationChannelCleanupFinalizer}
	r := newReconcilerWithObjects(t, ch)

	added, err := r.ensureFinalizer(context.Background(), ch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if added {
		t.Error("ensureFinalizer must not add a finalizer to a resource being deleted")
	}
}

// ---------------------------------------------------------------------------
// deleteConfigMap / deleteSecret
// ---------------------------------------------------------------------------

func TestDeleteConfigMap_NotFoundIsNotAnError(t *testing.T) {
	r := newReconcilerWithObjects(t)
	if err := r.deleteConfigMap(context.Background(), r.Client, "missing", "default"); err != nil {
		t.Errorf("deleteConfigMap should swallow NotFound; got %v", err)
	}
}

func TestDeleteConfigMap_DeletesExisting(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "ch1", Namespace: "default"},
	}
	r := newReconcilerWithObjects(t, cm)

	if err := r.deleteConfigMap(context.Background(), r.Client, "ch1", "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := r.Get(context.Background(), types.NamespacedName{Name: "ch1", Namespace: "default"}, &corev1.ConfigMap{})
	if err == nil {
		t.Error("expected ConfigMap to be deleted")
	}
}

func TestDeleteSecret_NotFoundIsNotAnError(t *testing.T) {
	r := newReconcilerWithObjects(t)
	if err := r.deleteSecret(context.Background(), r.Client, "missing", "default"); err != nil {
		t.Errorf("deleteSecret should swallow NotFound; got %v", err)
	}
}

func TestDeleteSecret_DeletesExisting(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ch1", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
	}
	r := newReconcilerWithObjects(t, sec)

	if err := r.deleteSecret(context.Background(), r.Client, "ch1", "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err := r.Get(context.Background(), types.NamespacedName{Name: "ch1", Namespace: "default"}, &corev1.Secret{})
	if err == nil {
		t.Error("expected Secret to be deleted")
	}
}
