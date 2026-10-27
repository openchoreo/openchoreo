// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/controller"
	esv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/externalsecrets/v1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	testNamespace = "default"
	testStore     = "default"
	testRemoteKey = "secret/default/generic/webhook-auth"
	testChannel   = "ch"
	testHeaderKey = "api-key"
)

var testRemoteKeys = RemoteKeyFormat{Prefix: "secret", Separator: "/"}

func newSecretReference(name string, entries map[string]string) *openchoreov1alpha1.SecretReference {
	sr := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
	}
	for secretKey, property := range entries {
		sr.Spec.Data = append(sr.Spec.Data, openchoreov1alpha1.SecretDataSource{
			SecretKey: secretKey,
			RemoteRef: openchoreov1alpha1.RemoteReference{Key: "secret/default/generic/" + name, Property: property},
		})
	}
	return sr
}

func newKubernetesSecret(name string, data map[string]string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Data:       map[string][]byte{},
	}
	for k, v := range data {
		secret.Data[k] = []byte(v)
	}
	return secret
}

func newWebhookChannelWithHeaderRef(name, secretName string) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	channel := newWebhookChannel(name, "dev")
	channel.Spec.WebhookConfig.Headers = map[string]openchoreov1alpha1.WebhookHeaderValue{
		"Content-Type": {Value: strPtr("application/json")},
		"X-API-Key": {ValueFrom: &openchoreov1alpha1.SecretValueFrom{
			SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: secretName, Key: testHeaderKey},
		}},
	}
	return channel
}

func newEmailChannelWithAuthRefs(username, password openchoreov1alpha1.SecretKeyRef) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	channel := newEmailChannel(testChannel, "dev")
	channel.Spec.EmailConfig.SMTP.Auth = &openchoreov1alpha1.SMTPAuth{
		Username: &openchoreov1alpha1.SecretValueFrom{SecretKeyRef: &username},
		Password: &openchoreov1alpha1.SecretValueFrom{SecretKeyRef: &password},
	}
	return channel
}

func newObservabilityPlaneResult(secretStore string) *controller.ObservabilityPlaneResult {
	op := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testNamespace},
	}
	if secretStore != "" {
		op.Spec.SecretStoreRef = &openchoreov1alpha1.SecretStoreRef{Name: secretStore}
	}
	return &controller.ObservabilityPlaneResult{ObservabilityPlane: op}
}

func requireInvalidSecrets(t *testing.T, err error, reason controller.ConditionReason) {
	t.Helper()
	invalid, ok := isInvalidSecretsError(err)
	if !ok {
		t.Fatalf("expected invalidSecretsError with reason %s, got %v", reason, err)
	}
	if invalid.reason != reason {
		t.Fatalf("reason = %s, want %s (message: %s)", invalid.reason, reason, invalid.message)
	}
}

// ---------------------------------------------------------------------------
// channelSecretRefs
// ---------------------------------------------------------------------------

func TestChannelSecretRefs_Email(t *testing.T) {
	channel := newEmailChannelWithAuthRefs(openchoreov1alpha1.SecretKeyRef{Name: "smtp", Key: "user"},
		openchoreov1alpha1.SecretKeyRef{Name: "smtp", Key: "pass"})

	refs := channelSecretRefs(channel)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0].secretKey != smtpAuthPasswordKey || refs[0].ref.Key != "pass" {
		t.Errorf("refs[0] = %+v", refs[0])
	}
	if refs[1].secretKey != smtpAuthUsernameKey || refs[1].ref.Key != "user" {
		t.Errorf("refs[1] = %+v", refs[1])
	}
}

func TestChannelSecretRefs_WebhookSkipsInlineHeaders(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")

	refs := channelSecretRefs(channel)
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	if refs[0].secretKey != "webhook.header.X-API-Key" || refs[0].ref.Name != "webhook-auth" {
		t.Errorf("refs[0] = %+v", refs[0])
	}
}

func TestChannelSecretRefs_EmailWithoutAuth(t *testing.T) {
	if refs := channelSecretRefs(newEmailChannel("ch", "dev")); len(refs) != 0 {
		t.Errorf("got %d refs, want 0", len(refs))
	}
}

// ---------------------------------------------------------------------------
// RemoteKeyFormat
// ---------------------------------------------------------------------------

func TestRemoteKeyFormat_NamespacePrefix(t *testing.T) {
	tests := []struct {
		format RemoteKeyFormat
		want   string
	}{
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, "secret/team-a/"},
		{RemoteKeyFormat{Prefix: "", Separator: "/"}, "team-a/"},
		{RemoteKeyFormat{Prefix: "//org///secrets/", Separator: "-"}, "org-secrets-team-a-"},
	}
	for _, tt := range tests {
		if got := tt.format.namespacePrefix("team-a"); got != tt.want {
			t.Errorf("%+v.namespacePrefix(team-a) = %q, want %q", tt.format, got, tt.want)
		}
	}
}

func TestRemoteKeyFormat_SecretKey(t *testing.T) {
	// The same keys as the secret API's remoteKeyFor writes.
	tests := []struct {
		format     RemoteKeyFormat
		secretType corev1.SecretType
		want       string
	}{
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, corev1.SecretTypeOpaque, "secret/team/generic/creds"},
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, "", "secret/team/generic/creds"},
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, corev1.SecretTypeBasicAuth, "secret/team/basic-auth/creds"},
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, corev1.SecretTypeSSHAuth, "secret/team/ssh-auth/creds"},
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, corev1.SecretTypeDockerConfigJson, "secret/team/registry/creds"},
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, corev1.SecretTypeTLS, "secret/team/tls/creds"},
		{RemoteKeyFormat{Prefix: "", Separator: "/"}, corev1.SecretTypeOpaque, "team/generic/creds"},
		{RemoteKeyFormat{Prefix: "oc/secrets", Separator: "-"}, corev1.SecretTypeOpaque, "oc-secrets-team-generic-creds"},
	}
	for _, tt := range tests {
		if got := tt.format.secretKey("team", tt.secretType, "creds"); got != tt.want {
			t.Errorf("%+v.secretKey(team, %q, creds) = %q, want %q", tt.format, tt.secretType, got, tt.want)
		}
	}
}

func TestRemoteKeyFormat_SharedWith(t *testing.T) {
	dash := RemoteKeyFormat{Prefix: "secret", Separator: "-"}
	tests := []struct {
		format    RemoteKeyFormat
		namespace string
		key       string
		want      string
	}{
		{dash, "team", "secret-team-generic-creds", ""},
		{dash, "team-a", "secret-team-a-generic-creds", ""},
		// Laid out as a key of both "team" (secret "generic-x") and "team-generic" (secret "x"):
		// ambiguous, whichever of the two namespaces asks, and whether or not the other one exists.
		{dash, "team", "secret-team-generic-generic-x", "team-generic"},
		{dash, "team-generic", "secret-team-generic-generic-x", "team"},
		{dash, "team", "secret-team-generic-registry-creds", "team-generic"},
		// Every key of "team-generic" is also laid out as one of "team" ("secret-team-" + "generic-...").
		{dash, "team-generic", "secret-team-generic-tls-cert", "team"},
		// So is every key of a namespace with a type segment in its name.
		{dash, "my-tls-team", "secret-my-tls-team-generic-creds", "my"},
		// "/" can't occur in namespace names, so a key belongs to one namespace only.
		{RemoteKeyFormat{Prefix: "secret", Separator: "/"}, "team", "secret/team/generic/generic/x", ""},
	}
	for _, tt := range tests {
		if got := tt.format.sharedWith(tt.namespace, tt.key); got != tt.want {
			t.Errorf("%+v.sharedWith(%s, %q) = %q, want %q", tt.format, tt.namespace, tt.key, got, tt.want)
		}
	}
}

func TestRemoteKeyFormat_KeyNamespaces(t *testing.T) {
	tests := []struct {
		format RemoteKeyFormat
		key    string
		want   []string
	}{
		{RemoteKeyFormat{Prefix: "secret", Separator: "-"}, "secret-team-generic-creds", []string{"team"}},
		{RemoteKeyFormat{Prefix: "secret", Separator: "-"}, "secret-team-generic-tls-cert", []string{"team", "team-generic"}},
		{RemoteKeyFormat{Prefix: "secret", Separator: "-"}, "secret-a-b-c-ssh-auth-k", []string{"a-b-c"}},
		// Only valid namespace names count.
		{RemoteKeyFormat{Prefix: "secret", Separator: "-"}, "secret-Team-generic-creds", nil},
		{RemoteKeyFormat{Prefix: "", Separator: "-"}, "team-generic-creds", []string{"team"}},
		{RemoteKeyFormat{Prefix: "secret", Separator: "-"}, "other-team-generic-creds", nil},
	}
	for _, tt := range tests {
		got := tt.format.keyNamespaces(tt.key)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%+v.keyNamespaces(%q) = %v, want %v", tt.format, tt.key, got, tt.want)
		}
	}
}

func TestRemoteKeyFormat_SeparatorInNamespaceNames(t *testing.T) {
	for sep, want := range map[string]bool{"/": false, "_": false, ".": false, "+": false, "=": false, "@": false, "-": true} {
		if got := (RemoteKeyFormat{Separator: sep}).separatorInNamespaceNames(); got != want {
			t.Errorf("separatorInNamespaceNames(%q) = %v, want %v", sep, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// resolveSecrets
// ---------------------------------------------------------------------------

func TestResolveSecrets_SecretReference(t *testing.T) {
	sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
	r := newReconcilerWithObjects(t, sr)

	resolved, err := r.resolveSecrets(context.Background(), newWebhookChannelWithHeaderRef("ch", "webhook-auth"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.source != secretSourceSecretReference {
		t.Fatalf("source = %q, want SecretReference", resolved.source)
	}
	if len(resolved.refs) != 1 || resolved.refs[0].remoteRef.Key != testRemoteKey || resolved.refs[0].remoteRef.Property != "api-key" {
		t.Errorf("refs = %+v", resolved.refs)
	}
}

func TestResolveSecrets_SecretReferenceTakesPrecedenceOverSecret(t *testing.T) {
	sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
	secret := newKubernetesSecret("webhook-auth", map[string]string{"api-key": "value"})
	r := newReconcilerWithObjects(t, sr, secret)

	resolved, err := r.resolveSecrets(context.Background(), newWebhookChannelWithHeaderRef("ch", "webhook-auth"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.source != secretSourceSecretReference {
		t.Errorf("source = %q, want SecretReference", resolved.source)
	}
}

func TestResolveSecrets_FallsBackToKubernetesSecret(t *testing.T) {
	secret := newKubernetesSecret("smtp", map[string]string{"user": "u", "pass": "p"})
	r := newReconcilerWithObjects(t, secret)

	channel := newEmailChannelWithAuthRefs(openchoreov1alpha1.SecretKeyRef{Name: "smtp", Key: "user"},
		openchoreov1alpha1.SecretKeyRef{Name: "smtp", Key: "pass"})
	resolved, err := r.resolveSecrets(context.Background(), channel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.source != secretSourceKubernetesSecret {
		t.Fatalf("source = %q, want Secret", resolved.source)
	}
	if len(resolved.secretNames) != 1 || resolved.secretNames[0] != "smtp" {
		t.Errorf("secretNames = %v, want [smtp]", resolved.secretNames)
	}
}

func TestResolveSecrets_NoReferences(t *testing.T) {
	r := newReconcilerWithObjects(t)
	resolved, err := r.resolveSecrets(context.Background(), newWebhookChannel("ch", "dev"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.source != secretSourceNone {
		t.Errorf("source = %q, want none", resolved.source)
	}
}

func TestResolveSecrets_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		objs    []client.Object
		channel *openchoreov1alpha1.ObservabilityAlertsNotificationChannel
		reason  controller.ConditionReason
	}{
		{
			name:    "neither SecretReference nor Secret",
			channel: newWebhookChannelWithHeaderRef("ch", "missing"),
			reason:  ReasonSecretNotFound,
		},
		{
			name:    "SecretReference without the key",
			objs:    []client.Object{newSecretReference("webhook-auth", map[string]string{"other": "other"})},
			channel: newWebhookChannelWithHeaderRef("ch", "webhook-auth"),
			reason:  ReasonSecretKeyNotFound,
		},
		{
			name:    "Secret without the key",
			objs:    []client.Object{newKubernetesSecret("webhook-auth", map[string]string{"other": "v"})},
			channel: newWebhookChannelWithHeaderRef("ch", "webhook-auth"),
			reason:  ReasonSecretKeyNotFound,
		},
		{
			name: "SecretReference and Secret mixed",
			objs: []client.Object{
				newSecretReference("smtp-user", map[string]string{"user": "user"}),
				newKubernetesSecret("smtp-pass", map[string]string{"pass": "p"}),
			},
			channel: newEmailChannelWithAuthRefs(openchoreov1alpha1.SecretKeyRef{Name: "smtp-user", Key: "user"},
				openchoreov1alpha1.SecretKeyRef{Name: "smtp-pass", Key: "pass"}),
			reason: ReasonMixedSecretSources,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcilerWithObjects(t, tt.objs...)
			_, err := r.resolveSecrets(context.Background(), tt.channel)
			requireInvalidSecrets(t, err, tt.reason)
		})
	}
}

// ---------------------------------------------------------------------------
// validateSecretReferences
// ---------------------------------------------------------------------------

func resolvedFor(sr *openchoreov1alpha1.SecretReference) *resolvedSecrets {
	remoteRef, _ := findRemoteRef(sr, testHeaderKey)
	return &resolvedSecrets{
		source: secretSourceSecretReference,
		refs: []resolvedSecretRef{{
			channelSecretRef: channelSecretRef{
				secretKey: "webhook.header.X-API-Key",
				ref:       &openchoreov1alpha1.SecretKeyRef{Name: sr.Name, Key: testHeaderKey},
			},
			secretReference: sr,
			remoteRef:       remoteRef,
		}},
	}
}

func TestValidateSecretReferences(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	r := &Reconciler{RemoteKeys: testRemoteKeys}

	t.Run("valid without targetPlane", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		if err := r.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid with the channel's observability plane as targetPlane", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		sr.Spec.TargetPlane = &openchoreov1alpha1.TargetPlaneRef{Kind: "ObservabilityPlane", Name: "default"}
		if err := r.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid with a ClusterObservabilityPlane as targetPlane", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		sr.Spec.TargetPlane = &openchoreov1alpha1.TargetPlaneRef{Kind: "ClusterObservabilityPlane", Name: "shared"}
		obsPlane := &controller.ObservabilityPlaneResult{ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "shared"},
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: testStore},
			},
		}}
		if err := r.validateSecretReferences(channel, obsPlane, resolvedFor(sr)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("observability plane without a secret store", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		err := r.validateSecretReferences(channel, newObservabilityPlaneResult(""), resolvedFor(sr))
		requireInvalidSecrets(t, err, ReasonSecretStoreNotConfigured)
	})

	t.Run("SecretReference pushed to another plane", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		sr.Spec.TargetPlane = &openchoreov1alpha1.TargetPlaneRef{Kind: "DataPlane", Name: "default"}
		err := r.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr))
		requireInvalidSecrets(t, err, ReasonSecretStoreUnreachable)
	})

	dashed := &Reconciler{RemoteKeys: RemoteKeyFormat{Prefix: "secret", Separator: "-"}}

	t.Run("dash separator rejects a longer namespace's keys", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		sr.Spec.Data[0].RemoteRef.Key = "secret-default-generic-webhook-auth"
		if err := dashed.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr)); err != nil {
			t.Errorf("own key: unexpected error: %v", err)
		}

		sr.Spec.Data[0].RemoteRef.Key = "secret-default-a-generic-webhook-auth"
		err := dashed.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr))
		requireInvalidSecrets(t, err, ReasonRemoteKeyNotAllowed)
	})

	t.Run("dash separator rejects a key a shorter namespace could also own", func(t *testing.T) {
		longChannel := channel.DeepCopy()
		longChannel.Namespace = "ncsr-generic"

		sr := newSecretReference("x", map[string]string{"api-key": "api-key"})
		sr.Namespace = longChannel.Namespace
		sr.Spec.Data[0].RemoteRef.Key = "secret-ncsr-generic-generic-x"
		err := dashed.validateSecretReferences(longChannel, newObservabilityPlaneResult(testStore), resolvedFor(sr))
		requireInvalidSecrets(t, err, ReasonRemoteKeyNotAllowed)
		if !strings.Contains(err.Error(), `as well as of namespace "ncsr"`) {
			t.Errorf("message %q should name the other namespace", err.Error())
		}
	})

	t.Run("dash separator rejects a key a namespace created later could own", func(t *testing.T) {
		// No namespace "default-generic" exists, but one created later could write this key.
		sr := newSecretReference("generic-x", map[string]string{"api-key": "api-key"})
		sr.Spec.Data[0].RemoteRef.Key = "secret-default-generic-generic-x"
		err := dashed.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr))
		requireInvalidSecrets(t, err, ReasonRemoteKeyNotAllowed)
		if !strings.Contains(err.Error(), `as well as of namespace "default-generic"`) {
			t.Errorf("message %q should name the other namespace", err.Error())
		}
	})

	t.Run("valid with the key of a typed secret", func(t *testing.T) {
		sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
		sr.Spec.Template.Type = corev1.SecretTypeBasicAuth
		sr.Spec.Data[0].RemoteRef.Key = "secret/default/basic-auth/webhook-auth"
		if err := r.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("remote key other than the SecretReference's own", func(t *testing.T) {
		for _, key := range []string{
			"opensearch-password",
			"secret/other/generic/webhook-auth",
			"secret/defaultx/generic/webhook-auth",
			// Path segments that a store might resolve to another namespace's key.
			"secret/default/../other/generic/webhook-auth",
			"secret/default/generic/../../other/generic/webhook-auth",
			"secret/default/./generic/webhook-auth",
			// The channel's namespace, but not this SecretReference's key.
			"secret/default/generic/other-secret",
			"secret/default/generic/webhook-auth/extra",
			"secret/default/tls/webhook-auth",
			"secret/default/my-app/webhook-auth",
		} {
			sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
			sr.Spec.Data[0].RemoteRef.Key = key
			err := r.validateSecretReferences(channel, newObservabilityPlaneResult(testStore), resolvedFor(sr))
			requireInvalidSecrets(t, err, ReasonRemoteKeyNotAllowed)
		}
	})
}

// ---------------------------------------------------------------------------
// buildExternalSecret and externalSecretReadiness
// ---------------------------------------------------------------------------

func TestBuildExternalSecret(t *testing.T) {
	channel := newEmailChannelWithAuthRefs(openchoreov1alpha1.SecretKeyRef{Name: "smtp", Key: "user"},
		openchoreov1alpha1.SecretKeyRef{Name: "smtp", Key: "pass"})
	slow := newSecretReference("smtp", map[string]string{"user": "username", "pass": "password"})
	slow.Spec.RefreshInterval = &metav1.Duration{Duration: time.Hour}
	fast := slow.DeepCopy()
	fast.Spec.RefreshInterval = &metav1.Duration{Duration: 5 * time.Minute}

	userRef, _ := findRemoteRef(slow, "user")
	passRef, _ := findRemoteRef(fast, "pass")
	passRef.Version = "3"
	resolved := &resolvedSecrets{
		source: secretSourceSecretReference,
		refs: []resolvedSecretRef{
			{channelSecretRef: channelSecretRef{secretKey: smtpAuthPasswordKey}, secretReference: fast, remoteRef: passRef},
			{channelSecretRef: channelSecretRef{secretKey: smtpAuthUsernameKey}, secretReference: slow, remoteRef: userRef},
		},
	}

	u, err := buildExternalSecret(channel, testStore, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.GetAPIVersion() != "external-secrets.io/v1" || u.GetKind() != "ExternalSecret" {
		t.Errorf("GVK = %s %s", u.GetAPIVersion(), u.GetKind())
	}
	if u.GetName() != "ch" || u.GetNamespace() != testNamespace {
		t.Errorf("name = %s/%s", u.GetNamespace(), u.GetName())
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(u.Object, "status"); found {
		t.Error("status must not be set")
	}

	es := &esv1.ExternalSecret{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, es); err != nil {
		t.Fatalf("FromUnstructured: %v", err)
	}
	if es.Spec.SecretStoreRef.Kind != "ClusterSecretStore" || es.Spec.SecretStoreRef.Name != testStore {
		t.Errorf("secretStoreRef = %+v", es.Spec.SecretStoreRef)
	}
	if es.Spec.RefreshInterval == nil || es.Spec.RefreshInterval.Duration != 5*time.Minute {
		t.Errorf("refreshInterval = %v, want the shortest of the SecretReferences (5m)", es.Spec.RefreshInterval)
	}
	if es.Spec.Target.Name != "ch" || es.Spec.Target.CreationPolicy != esv1.CreatePolicyOwner {
		t.Errorf("target = %+v", es.Spec.Target)
	}
	if es.Spec.Target.Template == nil || es.Spec.Target.Template.Metadata.Labels[labels.LabelKeyNotificationChannelName] != "ch" {
		t.Errorf("target template must carry the notification channel label, got %+v", es.Spec.Target.Template)
	}
	if es.Spec.Target.Template.Metadata.Labels["app.kubernetes.io/managed-by"] != "" {
		t.Error("the synced Secret must not carry the managed-by label of copied Secrets")
	}
	if len(es.Spec.Data) != 2 {
		t.Fatalf("data = %+v", es.Spec.Data)
	}
	if es.Spec.Data[0].SecretKey != smtpAuthPasswordKey || es.Spec.Data[0].RemoteRef.Property != "password" || es.Spec.Data[0].RemoteRef.Version != "3" {
		t.Errorf("data[0] = %+v", es.Spec.Data[0])
	}
	if es.Spec.Data[1].SecretKey != smtpAuthUsernameKey || es.Spec.Data[1].RemoteRef.Key != "secret/default/generic/smtp" {
		t.Errorf("data[1] = %+v", es.Spec.Data[1])
	}
}

func externalSecretWithReady(status corev1.ConditionStatus, message string) *unstructured.Unstructured {
	es := &esv1.ExternalSecret{ObjectMeta: metav1.ObjectMeta{Name: "ch", Namespace: testNamespace, Generation: 2}}
	if status != "" {
		es.Status.Conditions = []esv1.ExternalSecretStatusCondition{{Type: esv1.ExternalSecretReady, Status: status, Message: message}}
	}
	if status == corev1.ConditionTrue {
		es.Status.SyncedResourceVersion = "2-0123abcd"
	}
	obj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(es)
	return &unstructured.Unstructured{Object: obj}
}

func TestExternalSecretReadiness_IgnoresReadyOfAnEarlierGeneration(t *testing.T) {
	// Right after a spec change, the status can still hold the previous generation's successful sync.
	u := externalSecretWithReady(corev1.ConditionTrue, "")
	if err := unstructured.SetNestedField(u.Object, "1-0123abcd", "status", "syncedResourceVersion"); err != nil {
		t.Fatalf("set syncedResourceVersion: %v", err)
	}
	status, reason, _, err := externalSecretReadiness(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != metav1.ConditionUnknown || reason != ReasonExternalSecretPending {
		t.Errorf("got %s/%s, want Unknown/%s", status, reason, ReasonExternalSecretPending)
	}
}

func TestExternalSecretReadiness_ReportsAFailureOfAnyGeneration(t *testing.T) {
	// ESO only records the synced version on success, so a failure is reported without that check.
	u := externalSecretWithReady(corev1.ConditionFalse, "could not get secret data from provider")
	status, reason, _, err := externalSecretReadiness(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != metav1.ConditionFalse || reason != ReasonExternalSecretNotSynced {
		t.Errorf("got %s/%s, want False/%s", status, reason, ReasonExternalSecretNotSynced)
	}
}

func TestExternalSecretReadiness(t *testing.T) {
	tests := []struct {
		status     corev1.ConditionStatus
		wantStatus metav1.ConditionStatus
		wantReason controller.ConditionReason
	}{
		{corev1.ConditionTrue, metav1.ConditionTrue, ReasonExternalSecretSynced},
		{corev1.ConditionFalse, metav1.ConditionFalse, ReasonExternalSecretNotSynced},
		{"", metav1.ConditionUnknown, ReasonExternalSecretPending},
	}
	for _, tt := range tests {
		status, reason, message, err := externalSecretReadiness(externalSecretWithReady(tt.status, "could not get secret data from provider"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != tt.wantStatus || reason != tt.wantReason {
			t.Errorf("ES Ready %q: got %s/%s, want %s/%s", tt.status, status, reason, tt.wantStatus, tt.wantReason)
		}
		if tt.status == corev1.ConditionFalse && !strings.Contains(message, "could not get secret data from provider") {
			t.Errorf("message %q should carry the ExternalSecret's message", message)
		}
	}
}

// ---------------------------------------------------------------------------
// Reconcile
// ---------------------------------------------------------------------------

// reconcileFixture holds a reconciler whose observability plane is a separate fake cluster.
type reconcileFixture struct {
	r        *Reconciler
	cp       client.Client
	op       client.Client
	recorder *record.FakeRecorder
}

func newOPScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(corev1): %v", err)
	}
	if err := esv1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme(esv1): %v", err)
	}
	return s
}

func newReconcileFixture(t *testing.T, cpObjs []client.Object, opObjs ...client.Object) *reconcileFixture {
	t.Helper()
	env := &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "dev", Namespace: testNamespace}}
	dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testNamespace}}
	op := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testNamespace},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: testStore},
		},
	}

	s := newTestScheme(t)
	cp := fake.NewClientBuilder().WithScheme(s).
		WithObjects(append([]client.Object{env, dp, op}, cpObjs...)...).
		WithStatusSubresource(&openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}).
		Build()
	// Like the cluster gateway proxy client, report deleting an object that does not exist as a success.
	opClient := fake.NewClientBuilder().WithScheme(newOPScheme(t)).WithObjects(opObjs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if err := c.Delete(ctx, obj, opts...); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			},
		}).
		Build()

	provider := k8sMocks.NewMockObservabilityPlaneClientProvider(t)
	provider.EXPECT().ObservabilityPlaneClient(mock.Anything).Return(opClient, nil).Maybe()

	recorder := record.NewFakeRecorder(10)
	return &reconcileFixture{
		r: &Reconciler{
			Client:              cp,
			Scheme:              s,
			PlaneClientProvider: provider,
			Recorder:            recorder,
			RemoteKeys:          testRemoteKeys,
		},
		cp:       cp,
		op:       opClient,
		recorder: recorder,
	}
}

func (f *reconcileFixture) reconcile(t *testing.T) ctrl.Result {
	t.Helper()
	result, err := f.r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: testChannel, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return result
}

func (f *reconcileFixture) channel(t *testing.T) *openchoreov1alpha1.ObservabilityAlertsNotificationChannel {
	t.Helper()
	channel := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{}
	if err := f.cp.Get(context.Background(), types.NamespacedName{Name: testChannel, Namespace: testNamespace}, channel); err != nil {
		t.Fatalf("get channel: %v", err)
	}
	return channel
}

func (f *reconcileFixture) externalSecret(t *testing.T) (*unstructured.Unstructured, bool) {
	t.Helper()
	es := newExternalSecretObject()
	err := f.op.Get(context.Background(), types.NamespacedName{Name: testChannel, Namespace: testNamespace}, es)
	if apierrors.IsNotFound(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("get ExternalSecret: %v", err)
	}
	return es, true
}

func requireCondition(t *testing.T, channel *openchoreov1alpha1.ObservabilityAlertsNotificationChannel,
	ct controller.ConditionType, status metav1.ConditionStatus, reason controller.ConditionReason) {
	t.Helper()
	cond := meta.FindStatusCondition(channel.Status.Conditions, string(ct))
	if cond == nil {
		t.Fatalf("condition %s not set; conditions: %+v", ct, channel.Status.Conditions)
	}
	if cond.Status != status || cond.Reason != string(reason) {
		t.Fatalf("condition %s = %s/%s (%s), want %s/%s", ct, cond.Status, cond.Reason, cond.Message, status, reason)
	}
}

func setExternalSecretReady(t *testing.T, opClient client.Client, status corev1.ConditionStatus) {
	t.Helper()
	es := newExternalSecretObject()
	if err := opClient.Get(context.Background(), types.NamespacedName{Name: testChannel, Namespace: testNamespace}, es); err != nil {
		t.Fatalf("get ExternalSecret: %v", err)
	}
	conditions := []any{map[string]any{"type": "Ready", "status": string(status)}}
	if err := unstructured.SetNestedSlice(es.Object, conditions, "status", "conditions"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	if status == corev1.ConditionTrue {
		// Like ESO, record the synced version as "<generation>-<hash>".
		synced := fmt.Sprintf("%d-0123abcd", es.GetGeneration())
		if err := unstructured.SetNestedField(es.Object, synced, "status", "syncedResourceVersion"); err != nil {
			t.Fatalf("set syncedResourceVersion: %v", err)
		}
	}
	if err := opClient.Update(context.Background(), es); err != nil {
		t.Fatalf("update ExternalSecret: %v", err)
	}
}

func TestReconcile_SecretReferenceAppliesExternalSecret(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
	copied := newKubernetesSecret("ch", map[string]string{"webhook.header.X-API-Key": "old"})
	copied.Labels = channelResourceLabels(channel)
	f := newReconcileFixture(t, []client.Object{channel, sr}, copied)

	result := f.reconcile(t)
	if result.RequeueAfter != externalSecretPendingPollInterval {
		t.Errorf("RequeueAfter = %v, want %v while the ExternalSecret has not synced", result.RequeueAfter, externalSecretPendingPollInterval)
	}

	es, ok := f.externalSecret(t)
	if !ok {
		t.Fatal("ExternalSecret was not applied to the observability plane")
	}
	data, _, _ := unstructured.NestedSlice(es.Object, "spec", "data")
	if len(data) != 1 || data[0].(map[string]any)["secretKey"] != "webhook.header.X-API-Key" {
		t.Errorf("ExternalSecret data = %v", data)
	}
	if err := f.op.Get(context.Background(), types.NamespacedName{Name: "ch", Namespace: testNamespace}, &corev1.Secret{}); !apierrors.IsNotFound(err) {
		t.Errorf("the copied Secret should be deleted so the ExternalSecret can own the name, got %v", err)
	}
	cm := &corev1.ConfigMap{}
	if err := f.op.Get(context.Background(), types.NamespacedName{Name: "ch", Namespace: testNamespace}, cm); err != nil {
		t.Errorf("ConfigMap was not applied: %v", err)
	}

	got := f.channel(t)
	requireCondition(t, got, ConditionSecretsReady, metav1.ConditionUnknown, ReasonExternalSecretPending)
	requireCondition(t, got, ConditionReady, metav1.ConditionFalse, ReasonExternalSecretPending)

	setExternalSecretReady(t, f.op, corev1.ConditionTrue)
	// ExternalSecrets aren't watched, so a synced channel is rechecked now and then.
	if result := f.reconcile(t); result.RequeueAfter != externalSecretRecheckInterval {
		t.Errorf("RequeueAfter = %v, want %v once synced", result.RequeueAfter, externalSecretRecheckInterval)
	}
	got = f.channel(t)
	requireCondition(t, got, ConditionSecretsReady, metav1.ConditionTrue, ReasonExternalSecretSynced)
	requireCondition(t, got, ConditionReady, metav1.ConditionTrue, ReasonReconciled)
	if got.Status.ObservedGeneration != got.Generation {
		t.Errorf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
	}
}

func TestReconcile_ExternalSecretNotSynced(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	sr := newSecretReference("webhook-auth", map[string]string{"api-key": "api-key"})
	f := newReconcileFixture(t, []client.Object{channel, sr})

	f.reconcile(t)
	setExternalSecretReady(t, f.op, corev1.ConditionFalse)
	result := f.reconcile(t)
	if result.RequeueAfter != externalSecretPollInterval {
		t.Errorf("RequeueAfter = %v, want %v", result.RequeueAfter, externalSecretPollInterval)
	}
	got := f.channel(t)
	requireCondition(t, got, ConditionSecretsReady, metav1.ConditionFalse, ReasonExternalSecretNotSynced)
	requireCondition(t, got, ConditionReady, metav1.ConditionFalse, ReasonExternalSecretNotSynced)
}

func TestReconcile_KubernetesSecretFallback(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	secret := newKubernetesSecret("webhook-auth", map[string]string{"api-key": " s3cret\n"})
	f := newReconcileFixture(t, []client.Object{channel, secret})

	if result := f.reconcile(t); result != (ctrl.Result{}) {
		t.Errorf("result = %+v, want no requeue", result)
	}

	copied := &corev1.Secret{}
	if err := f.op.Get(context.Background(), types.NamespacedName{Name: "ch", Namespace: testNamespace}, copied); err != nil {
		t.Fatalf("copied Secret was not applied: %v", err)
	}
	if string(copied.Data["webhook.header.X-API-Key"]) != "s3cret" {
		t.Errorf("copied value = %q", copied.Data["webhook.header.X-API-Key"])
	}
	if _, ok := f.externalSecret(t); ok {
		t.Error("no ExternalSecret should be applied for Kubernetes Secret references")
	}

	got := f.channel(t)
	requireCondition(t, got, ConditionSecretsReady, metav1.ConditionTrue, ReasonDeprecatedKubernetesSecret)
	requireCondition(t, got, ConditionReady, metav1.ConditionTrue, ReasonReconciled)

	select {
	case event := <-f.recorder.Events:
		if !strings.Contains(event, "Warning "+string(ReasonDeprecatedKubernetesSecret)) {
			t.Errorf("event = %q, want a deprecation warning", event)
		}
	default:
		t.Error("expected a deprecation warning event")
	}
}

func TestReconcile_NoSecretReferences(t *testing.T) {
	channel := newWebhookChannel("ch", "dev")
	f := newReconcileFixture(t, []client.Object{channel})

	f.reconcile(t)
	if err := f.op.Get(context.Background(), types.NamespacedName{Name: "ch", Namespace: testNamespace}, &corev1.Secret{}); err != nil {
		t.Errorf("the observer needs a Secret even without references: %v", err)
	}
	got := f.channel(t)
	requireCondition(t, got, ConditionSecretsReady, metav1.ConditionTrue, ReasonNoSecretsReferenced)
	requireCondition(t, got, ConditionReady, metav1.ConditionTrue, ReasonReconciled)
	if len(f.recorder.Events) != 0 {
		t.Errorf("unexpected event: %s", <-f.recorder.Events)
	}
}

func TestReconcile_SwitchBackToKubernetesSecretWaitsForSyncedSecret(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	secret := newKubernetesSecret("webhook-auth", map[string]string{"api-key": "v"})
	es := newExternalSecretObject()
	es.SetName("ch")
	es.SetNamespace(testNamespace)
	es.SetLabels(map[string]string{"app.kubernetes.io/managed-by": managedByLabelValue})
	synced := newKubernetesSecret("ch", map[string]string{"webhook.header.X-API-Key": "old"})
	synced.OwnerReferences = []metav1.OwnerReference{{APIVersion: "external-secrets.io/v1", Kind: "ExternalSecret", Name: "ch", UID: "uid"}}
	f := newReconcileFixture(t, []client.Object{channel, secret}, es, synced)

	result := f.reconcile(t)
	if result.RequeueAfter != secretReplacementPollInterval {
		t.Errorf("RequeueAfter = %v, want %v", result.RequeueAfter, secretReplacementPollInterval)
	}
	if _, ok := f.externalSecret(t); ok {
		t.Error("the ExternalSecret should be deleted")
	}
	requireCondition(t, f.channel(t), ConditionSecretsReady, metav1.ConditionUnknown, ReasonReplacingSecret)

	// The fake cluster has no garbage collector; remove the synced Secret as it would.
	if err := f.op.Delete(context.Background(), synced); err != nil {
		t.Fatalf("delete synced Secret: %v", err)
	}
	f.reconcile(t)
	requireCondition(t, f.channel(t), ConditionSecretsReady, metav1.ConditionTrue, ReasonDeprecatedKubernetesSecret)
}

func TestReconcile_InvalidReferenceIsReportedNotRetried(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "missing")
	f := newReconcileFixture(t, []client.Object{channel})

	if result := f.reconcile(t); result != (ctrl.Result{}) {
		t.Errorf("result = %+v, want no requeue", result)
	}
	got := f.channel(t)
	requireCondition(t, got, ConditionSecretsReady, metav1.ConditionFalse, ReasonSecretNotFound)
	requireCondition(t, got, ConditionReady, metav1.ConditionFalse, ReasonSecretNotFound)
}

func TestReconcile_PlaneConfigurationProblemsArePolled(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef(testChannel, "webhook-auth")
	sr := newSecretReference("webhook-auth", map[string]string{testHeaderKey: testHeaderKey})
	sr.Spec.TargetPlane = &openchoreov1alpha1.TargetPlaneRef{Kind: "DataPlane", Name: "default"}
	f := newReconcileFixture(t, []client.Object{channel, sr})

	// The observability plane is not watched, so a fix to it is only noticed by polling.
	if result := f.reconcile(t); result.RequeueAfter != planeConfigPollInterval {
		t.Errorf("RequeueAfter = %v, want %v", result.RequeueAfter, planeConfigPollInterval)
	}
	requireCondition(t, f.channel(t), ConditionReady, metav1.ConditionFalse, ReasonSecretStoreUnreachable)
}

func TestReconcile_RemoteKeyOutsideNamespaceIsRejected(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	sr := newSecretReference("webhook-auth", map[string]string{"api-key": "value"})
	sr.Spec.Data[0].RemoteRef.Key = "observer-oauth-client-secret"
	f := newReconcileFixture(t, []client.Object{channel, sr})

	f.reconcile(t)
	if _, ok := f.externalSecret(t); ok {
		t.Error("no ExternalSecret should be applied for a remote key outside the namespace")
	}
	requireCondition(t, f.channel(t), ConditionReady, metav1.ConditionFalse, ReasonRemoteKeyNotAllowed)
}

func TestReconcile_InvalidReferenceRemovesTheExternalSecret(t *testing.T) {
	tests := []struct {
		name       string
		invalidate func(t *testing.T, cp client.Client, sr *openchoreov1alpha1.SecretReference)
		reason     controller.ConditionReason
	}{
		{
			name: "remote key no longer allowed",
			invalidate: func(t *testing.T, cp client.Client, sr *openchoreov1alpha1.SecretReference) {
				sr.Spec.Data[0].RemoteRef.Key = "observer-oauth-client-secret"
				if err := cp.Update(context.Background(), sr); err != nil {
					t.Fatalf("update SecretReference: %v", err)
				}
			},
			reason: ReasonRemoteKeyNotAllowed,
		},
		{
			name: "SecretReference deleted",
			invalidate: func(t *testing.T, cp client.Client, sr *openchoreov1alpha1.SecretReference) {
				if err := cp.Delete(context.Background(), sr); err != nil {
					t.Fatalf("delete SecretReference: %v", err)
				}
			},
			reason: ReasonSecretNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := newWebhookChannelWithHeaderRef(testChannel, "webhook-auth")
			sr := newSecretReference("webhook-auth", map[string]string{testHeaderKey: testHeaderKey})
			f := newReconcileFixture(t, []client.Object{channel, sr})

			f.reconcile(t)
			if _, ok := f.externalSecret(t); !ok {
				t.Fatal("ExternalSecret was not applied to the observability plane")
			}

			current := &openchoreov1alpha1.SecretReference{}
			if err := f.cp.Get(context.Background(), client.ObjectKeyFromObject(sr), current); err != nil {
				t.Fatalf("get SecretReference: %v", err)
			}
			tt.invalidate(t, f.cp, current)
			f.reconcile(t)
			if _, ok := f.externalSecret(t); ok {
				t.Error("the ExternalSecret should be deleted once the channel's references are invalid")
			}
			requireCondition(t, f.channel(t), ConditionReady, metav1.ConditionFalse, tt.reason)
		})
	}
}

func newForeignExternalSecret() *unstructured.Unstructured {
	es := newExternalSecretObject()
	es.SetName(testChannel)
	es.SetNamespace(testNamespace)
	es.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "someone-else"})
	return es
}

func TestReconcile_DoesNotTakeOverAForeignExternalSecret(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef(testChannel, "webhook-auth")
	sr := newSecretReference("webhook-auth", map[string]string{testHeaderKey: testHeaderKey})
	f := newReconcileFixture(t, []client.Object{channel, sr}, newForeignExternalSecret())

	if result := f.reconcile(t); result.RequeueAfter != planeConfigPollInterval {
		t.Errorf("RequeueAfter = %v, want %v", result.RequeueAfter, planeConfigPollInterval)
	}
	requireCondition(t, f.channel(t), ConditionReady, metav1.ConditionFalse, ReasonExternalSecretConflict)
	es, ok := f.externalSecret(t)
	if !ok || es.GetLabels()["app.kubernetes.io/managed-by"] != "someone-else" {
		t.Error("the foreign ExternalSecret must be left as it is")
	}
	if _, found, _ := unstructured.NestedSlice(es.Object, "spec", "data"); found {
		t.Error("the foreign ExternalSecret's spec must not be overwritten")
	}
}

func TestReconcile_KubernetesSecretFallbackLeavesAForeignExternalSecret(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef(testChannel, "webhook-auth")
	secret := newKubernetesSecret("webhook-auth", map[string]string{testHeaderKey: "v"})
	f := newReconcileFixture(t, []client.Object{channel, secret}, newForeignExternalSecret())

	f.reconcile(t)
	if _, ok := f.externalSecret(t); !ok {
		t.Error("the foreign ExternalSecret must not be deleted")
	}
	requireCondition(t, f.channel(t), ConditionReady, metav1.ConditionFalse, ReasonExternalSecretConflict)
}

func TestFinalize_LeavesAForeignExternalSecret(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef(testChannel, "webhook-auth")
	channel.Finalizers = []string{NotificationChannelCleanupFinalizer}
	now := metav1.Now()
	channel.DeletionTimestamp = &now
	f := newReconcileFixture(t, []client.Object{channel}, newForeignExternalSecret())

	f.reconcile(t)
	if _, ok := f.externalSecret(t); !ok {
		t.Error("the foreign ExternalSecret must not be deleted on finalization")
	}
}

func TestFinalize_DeletesExternalSecret(t *testing.T) {
	channel := newWebhookChannelWithHeaderRef("ch", "webhook-auth")
	channel.Finalizers = []string{NotificationChannelCleanupFinalizer}
	now := metav1.Now()
	channel.DeletionTimestamp = &now
	es := newExternalSecretObject()
	es.SetName("ch")
	es.SetNamespace(testNamespace)
	es.SetLabels(map[string]string{"app.kubernetes.io/managed-by": managedByLabelValue})
	f := newReconcileFixture(t, []client.Object{channel}, es)

	f.reconcile(t)
	if _, ok := f.externalSecret(t); ok {
		t.Error("the ExternalSecret should be deleted on finalization")
	}
}

// ---------------------------------------------------------------------------
// findChannelsReferencing
// ---------------------------------------------------------------------------

func TestFindChannelsReferencing(t *testing.T) {
	referencing := newWebhookChannelWithHeaderRef("referencing", "webhook-auth")
	other := newWebhookChannelWithHeaderRef("other", "other-auth")
	inline := newWebhookChannel("inline", "dev")
	r := newReconcilerWithObjects(t, referencing, other, inline)

	for _, obj := range []client.Object{
		newSecretReference("webhook-auth", nil),
		newKubernetesSecret("webhook-auth", nil),
	} {
		requests := r.findChannelsReferencing(context.Background(), obj)
		if len(requests) != 1 || requests[0].Name != "referencing" {
			t.Errorf("%T: requests = %v, want [referencing]", obj, requests)
		}
	}

	elsewhere := newSecretReference("webhook-auth", nil)
	elsewhere.Namespace = "other-namespace"
	if requests := r.findChannelsReferencing(context.Background(), elsewhere); len(requests) != 0 {
		t.Errorf("requests = %v, want none for another namespace", requests)
	}
}
