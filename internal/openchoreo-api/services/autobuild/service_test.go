// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

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

	t.Run("success with bitbucket token validation", func(t *testing.T) {
		secret := newWebhookSecret("bitbucket-secret", "my-token")
		processor := &mockProcessor{components: []string{"comp-a", "comp-b"}}
		svc := newService(t, processor, secret)

		result, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hook-UUID",
			Signature:       "my-token",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{"push":{}}`),
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

	t.Run("secret key missing with signature header", func(t *testing.T) {
		// Secret exists but missing the expected key; signature header is present so allowEmpty=false
		secret := newWebhookSecret("other-key", "value")
		svc := newService(t, &mockProcessor{}, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hook-UUID",
			Signature:       "token",
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
			SignatureHeader: "X-Hook-UUID",
			Signature:       "wrong-token",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{}`),
		})

		require.ErrorIs(t, err, ErrInvalidSignature)
	})

	t.Run("processor error", func(t *testing.T) {
		secret := newWebhookSecret("bitbucket-secret", "my-token")
		processor := &mockProcessor{err: fmt.Errorf("build trigger failed")}
		svc := newService(t, processor, secret)

		_, err := svc.ProcessWebhook(ctx, &ProcessWebhookParams{
			ProviderType:    git.ProviderBitbucket,
			SignatureHeader: "X-Hook-UUID",
			Signature:       "my-token",
			SecretKey:       "bitbucket-secret",
			Payload:         []byte(`{}`),
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process webhook")
	})
}

func TestGetWebhookSecret(t *testing.T) {
	ctx := context.Background()

	t.Run("returns secret value", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("my-key", "my-value"))

		val, err := svc.getWebhookSecret(ctx, "my-key", false)
		require.NoError(t, err)
		assert.Equal(t, "my-value", val)
	})

	t.Run("missing key with allowEmpty returns empty", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("other-key", "value"))

		val, err := svc.getWebhookSecret(ctx, "missing-key", true)
		require.NoError(t, err)
		assert.Equal(t, "", val)
	})

	t.Run("missing key without allowEmpty returns error", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("other-key", "value"))

		_, err := svc.getWebhookSecret(ctx, "missing-key", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain")
	})

	t.Run("empty value with allowEmpty returns empty", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("my-key", ""))

		val, err := svc.getWebhookSecret(ctx, "my-key", true)
		require.NoError(t, err)
		assert.Equal(t, "", val)
	})

	t.Run("empty value without allowEmpty returns error", func(t *testing.T) {
		svc := newAutobuildService(t, newWebhookSecret("my-key", ""))

		_, err := svc.getWebhookSecret(ctx, "my-key", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("secret not found returns error", func(t *testing.T) {
		svc := newAutobuildService(t)

		_, err := svc.getWebhookSecret(ctx, "any-key", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get webhook secret")
	})
}
