// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// hmacSignature returns the "sha256=<hex>" HMAC-SHA256 signature of payload under secret,
// matching the format git providers send in their signature headers.
func hmacSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// mockProcessor is a test double for WebhookProcessor.
type mockProcessor struct {
	components []string
	err        error
}

func (m *mockProcessor) ProcessWebhook(_ context.Context, _ git.Provider, _ []byte) ([]string, error) {
	return m.components, m.err
}

func newService(t *testing.T, processor WebhookProcessor, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), processor, testutil.TestLogger())
}

func newWebhookSecret(key, value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookSecretName,
			Namespace: webhookSecretNamespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
}

func newAutobuildService(t *testing.T, objs ...client.Object) *autobuildService {
	t.Helper()
	return &autobuildService{
		k8sClient: testutil.NewFakeClient(objs...),
		logger:    testutil.TestLogger(),
	}
}

func TestProcessWebhook(t *testing.T) {
	ctx := context.Background()

	t.Run("success with bitbucket HMAC signature", func(t *testing.T) {
		const secretValue = "my-token"
		payload := []byte(`{"push":{}}`)
		secret := newWebhookSecret("bitbucket-secret", secretValue)
		processor := &mockProcessor{components: []string{"comp-a", "comp-b"}}
		svc := newService(t, processor, secret)

		result, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       hmacSignature(secretValue, payload),
			SecretKey:       "bitbucket-secret",
			Payload:         payload,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, []string{"comp-a", "comp-b"}, result.AffectedComponents)
	})

	t.Run("invalid provider type", func(t *testing.T) {
		svc := newService(t, &mockProcessor{})

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType: "unsupported",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get git provider")
	})

	t.Run("secret not found", func(t *testing.T) {
		// No secret object in the fake client
		svc := newService(t, &mockProcessor{})

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hook-UUID",
			Signature:       "token",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{}`),
		})

		require.ErrorIs(t, err, ErrSecretNotConfigured)
	})

	t.Run("secret key missing fails closed", func(t *testing.T) {
		// Secret object exists but is missing the expected provider key.
		secret := newWebhookSecret("other-key", "value")
		svc := newService(t, &mockProcessor{}, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       "sha256=abc",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{}`),
		})

		require.ErrorIs(t, err, ErrSecretNotConfigured)
	})

	t.Run("invalid signature", func(t *testing.T) {
		secret := newWebhookSecret("bitbucket-secret", "correct-token")
		svc := newService(t, &mockProcessor{}, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       "sha256=deadbeef",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{}`),
		})

		require.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("missing signature fails closed", func(t *testing.T) {
		// A forged Bitbucket webhook with a configured secret but no signature must be rejected.
		secret := newWebhookSecret("bitbucket-secret", "correct-token")
		svc := newService(t, &mockProcessor{}, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       "",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{}`),
		})

		require.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("processor error", func(t *testing.T) {
		const secretValue = "my-token"
		payload := []byte(`{}`)
		secret := newWebhookSecret("bitbucket-secret", secretValue)
		processor := &mockProcessor{err: fmt.Errorf("build trigger failed")}
		svc := newService(t, processor, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       hmacSignature(secretValue, payload),
			SecretKey:       "bitbucket-secret",
			Payload:         payload,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process webhook")
	})
}

func TestGetWebhookSecret(t *testing.T) {
	ctx := context.Background()

	t.Run("returns secret value", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("my-key", "my-value"))

		val, err := svc.getWebhookSecret(ctx, "my-key")
		require.NoError(t, err)
		assert.Equal(t, "my-value", val)
	})

	t.Run("missing key returns ErrSecretNotConfigured", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("other-key", "value"))

		_, err := svc.getWebhookSecret(ctx, "missing-key")
		require.ErrorIs(t, err, ErrSecretNotConfigured)
	})

	t.Run("empty value returns ErrSecretNotConfigured", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("my-key", ""))

		_, err := svc.getWebhookSecret(ctx, "my-key")
		require.ErrorIs(t, err, ErrSecretNotConfigured)
	})

	t.Run("secret not found returns ErrSecretNotConfigured", func(t *testing.T) {
		svc := newAutobuildService(t)

		_, err := svc.getWebhookSecret(ctx, "any-key")
		require.ErrorIs(t, err, ErrSecretNotConfigured)
	})
}

// TestForgedBitbucketWebhookRejected exercises the scenario where an unauthenticated caller
// selects the Bitbucket provider via request headers to reach a validation path that must not
// accept a request without a valid HMAC signature. The forged payload targets a GitHub-hosted
// repository (the provider is attacker-selected), and every variant must be rejected before any
// build is triggered.
func TestForgedBitbucketWebhookRejected(t *testing.T) {
	ctx := context.Background()
	forgedPayload := []byte(`{"push":{"changes":[{"new":{"name":"main","type":"branch"},` +
		`"commits":[{"hash":"0123456789abcdef0123456789abcdef01234567"}]}]},` +
		`"repository":{"links":{"html":{"href":"https://github.com/target-org/target-repo"}}}}`)

	t.Run("deployment without bitbucket-secret fails closed", func(t *testing.T) {
		// A GitHub-only deployment: only github-secret is configured, no bitbucket-secret.
		secret := newWebhookSecret("github-secret", "gh-hmac-secret")
		processor := &mockProcessor{components: []string{"must-not-trigger"}}
		svc := newService(t, processor, secret)

		result, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       "", // attacker sends no signature
			SecretKey:       "bitbucket-secret",
			Payload:         forgedPayload,
		})

		require.ErrorIs(t, err, ErrSecretNotConfigured)
		require.Nil(t, result)
	})

	t.Run("bitbucket-secret configured but no signature fails closed", func(t *testing.T) {
		secret := newWebhookSecret("bitbucket-secret", "bb-hmac-secret")
		processor := &mockProcessor{components: []string{"must-not-trigger"}}
		svc := newService(t, processor, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       "",
			SecretKey:       "bitbucket-secret",
			Payload:         forgedPayload,
		})

		require.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("bitbucket-secret configured with signature under a guessed secret fails closed", func(t *testing.T) {
		secret := newWebhookSecret("bitbucket-secret", "bb-hmac-secret")
		svc := newService(t, &mockProcessor{}, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hub-Signature",
			Signature:       hmacSignature("attacker-guessed-secret", forgedPayload),
			SecretKey:       "bitbucket-secret",
			Payload:         forgedPayload,
		})

		require.ErrorIs(t, err, ErrInvalidSignature)
	})
}

// TestGitHubWebhookSignatureEnforced is a control asserting the GitHub path rejects an invalid
// signature, confirming Bitbucket is now held to the same standard rather than being a weaker path.
func TestGitHubWebhookSignatureEnforced(t *testing.T) {
	ctx := context.Background()
	secret := newWebhookSecret("github-secret", "gh-hmac-secret")
	svc := newService(t, &mockProcessor{}, secret)

	_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
		ProviderType:    git.ProviderGitHub,
		SignatureHeader: "X-Hub-Signature-256",
		Signature:       "sha256=deadbeef",
		SecretKey:       "github-secret",
		Payload:         []byte(`{"ref":"refs/heads/main"}`),
	})

	require.ErrorIs(t, err, ErrInvalidSignature)
}
