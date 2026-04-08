// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add openchoreo scheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	return s
}

const testNamespace = "ns1"

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- validateCredentials tests ---

func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name    string
		params  *CreateGitSecretParams
		wantErr bool
		errMsg  string
	}{
		{
			name: "basic-auth with token succeeds",
			params: &CreateGitSecretParams{
				SecretType: "basic-auth",
				Token:      "ghp_abc123",
			},
			wantErr: false,
		},
		{
			name: "basic-auth without token fails",
			params: &CreateGitSecretParams{
				SecretType: "basic-auth",
			},
			wantErr: true,
			errMsg:  "token is required for basic-auth type",
		},
		{
			name: "ssh-auth with key succeeds",
			params: &CreateGitSecretParams{
				SecretType: "ssh-auth",
				SSHKey:     "-----BEGIN OPENSSH PRIVATE KEY-----",
			},
			wantErr: false,
		},
		{
			name: "ssh-auth without key fails",
			params: &CreateGitSecretParams{
				SecretType: "ssh-auth",
			},
			wantErr: true,
			errMsg:  "sshKey is required for ssh-auth type",
		},
		{
			name: "unsupported secret type fails",
			params: &CreateGitSecretParams{
				SecretType: "unknown",
			},
			wantErr: true,
			errMsg:  "unsupported secret type: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentials(tt.params)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var valErr *services.ValidationError
				if ok := isValidationError(err, &valErr); !ok {
					t.Fatalf("expected ValidationError, got %T: %v", err, err)
				}
				if valErr.Msg != tt.errMsg {
					t.Errorf("error message = %q, want %q", valErr.Msg, tt.errMsg)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func isValidationError(err error, target **services.ValidationError) bool {
	v := &services.ValidationError{}
	ok := errors.As(err, &v)
	if ok {
		*target = v
	}
	return ok
}

// --- buildSecretReference tests ---

func TestBuildSecretReference(t *testing.T) {
	svc := &gitSecretService{logger: newTestLogger()}

	tests := []struct {
		name       string
		namespace  string
		secretName string
		secretType string
		username   string
		sshKeyID   string
		wpKind     string
		wpName     string
	}{
		{
			name:       "basic-auth with WorkflowPlane",
			namespace:  testNamespace,
			secretName: "my-secret",
			secretType: "basic-auth",
			username:   "user1",
			wpKind:     workflowPlaneKindWorkflowPlane,
			wpName:     "wp-default",
		},
		{
			name:       "ssh-auth with ClusterWorkflowPlane",
			namespace:  "ns2",
			secretName: "ssh-secret",
			secretType: "ssh-auth",
			sshKeyID:   "key-id-123",
			wpKind:     workflowPlaneKindClusterWorkflowPlane,
			wpName:     "cwp-shared",
		},
		{
			name:       "basic-auth without username",
			namespace:  testNamespace,
			secretName: "token-only",
			secretType: "basic-auth",
			wpKind:     workflowPlaneKindWorkflowPlane,
			wpName:     "wp1",
		},
		{
			name:       "ssh-auth without key ID",
			namespace:  testNamespace,
			secretName: "ssh-only",
			secretType: "ssh-auth",
			wpKind:     workflowPlaneKindClusterWorkflowPlane,
			wpName:     "cwp1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := svc.buildSecretReference(tt.namespace, tt.secretName, tt.secretType, tt.username, tt.sshKeyID, tt.wpKind, tt.wpName)

			// Verify metadata
			if ref.Name != tt.secretName {
				t.Errorf("Name = %q, want %q", ref.Name, tt.secretName)
			}
			if ref.Namespace != tt.namespace {
				t.Errorf("Namespace = %q, want %q", ref.Namespace, tt.namespace)
			}

			// Verify labels
			labels := ref.Labels
			if labels[gitSecretTypeLabel] != gitSecretTypeValue {
				t.Errorf("label %s = %q, want %q", gitSecretTypeLabel, labels[gitSecretTypeLabel], gitSecretTypeValue)
			}
			if labels[gitSecretAuthTypeLabel] != tt.secretType {
				t.Errorf("label %s = %q, want %q", gitSecretAuthTypeLabel, labels[gitSecretAuthTypeLabel], tt.secretType)
			}
			if labels[workflowPlaneKindLabel] != tt.wpKind {
				t.Errorf("label %s = %q, want %q", workflowPlaneKindLabel, labels[workflowPlaneKindLabel], tt.wpKind)
			}
			if labels[workflowPlaneNameLabel] != tt.wpName {
				t.Errorf("label %s = %q, want %q", workflowPlaneNameLabel, labels[workflowPlaneNameLabel], tt.wpName)
			}

			// Verify no annotations are set
			if len(ref.Annotations) != 0 {
				t.Errorf("expected no annotations, got %v", ref.Annotations)
			}

			// Verify data sources
			if tt.secretType == "basic-auth" {
				if ref.Spec.Template.Type != corev1.SecretTypeBasicAuth {
					t.Errorf("Template.Type = %v, want %v", ref.Spec.Template.Type, corev1.SecretTypeBasicAuth)
				}
				expectKeys := 1
				if tt.username != "" {
					expectKeys = 2
				}
				if len(ref.Spec.Data) != expectKeys {
					t.Errorf("Data length = %d, want %d", len(ref.Spec.Data), expectKeys)
				}
			} else {
				if ref.Spec.Template.Type != corev1.SecretTypeSSHAuth {
					t.Errorf("Template.Type = %v, want %v", ref.Spec.Template.Type, corev1.SecretTypeSSHAuth)
				}
				expectKeys := 1
				if tt.sshKeyID != "" {
					expectKeys = 2
				}
				if len(ref.Spec.Data) != expectKeys {
					t.Errorf("Data length = %d, want %d", len(ref.Spec.Data), expectKeys)
				}
			}
		})
	}
}

// --- buildGitSecret tests ---

func TestBuildGitSecret(t *testing.T) {
	svc := &gitSecretService{logger: newTestLogger()}

	tests := []struct {
		name       string
		secretType string
		username   string
		token      string
		sshKey     string
		sshKeyID   string
		wantType   corev1.SecretType
		wantKeys   []string
	}{
		{
			name:       "basic-auth with username",
			secretType: "basic-auth",
			username:   "user1",
			token:      "tok123",
			wantType:   corev1.SecretTypeBasicAuth,
			wantKeys:   []string{"password", "username"},
		},
		{
			name:       "basic-auth without username",
			secretType: "basic-auth",
			token:      "tok123",
			wantType:   corev1.SecretTypeBasicAuth,
			wantKeys:   []string{"password"},
		},
		{
			name:       "ssh-auth with key ID",
			secretType: "ssh-auth",
			sshKey:     "-----BEGIN KEY-----",
			sshKeyID:   "key-id",
			wantType:   corev1.SecretTypeSSHAuth,
			wantKeys:   []string{"ssh-privatekey", "ssh-key-id"},
		},
		{
			name:       "ssh-auth without key ID",
			secretType: "ssh-auth",
			sshKey:     "-----BEGIN KEY-----",
			wantType:   corev1.SecretTypeSSHAuth,
			wantKeys:   []string{"ssh-privatekey"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := svc.buildGitSecret("test-secret", testNamespace, "workflows-ns1", tt.secretType, tt.username, tt.token, tt.sshKey, tt.sshKeyID)

			if secret.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", secret.Type, tt.wantType)
			}
			if secret.Labels[ownerNamespaceLabel] != testNamespace {
				t.Errorf("owner namespace label = %q, want %q", secret.Labels[ownerNamespaceLabel], testNamespace)
			}
			if len(secret.StringData) != len(tt.wantKeys) {
				t.Errorf("StringData has %d keys, want %d", len(secret.StringData), len(tt.wantKeys))
			}
			for _, key := range tt.wantKeys {
				if _, ok := secret.StringData[key]; !ok {
					t.Errorf("StringData missing key %q", key)
				}
			}
		})
	}
}

// --- ListGitSecrets tests ---

func TestListGitSecrets(t *testing.T) {
	scheme := newTestScheme(t)

	gitSecretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "git-secret-1",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				gitSecretAuthTypeLabel: "basic-auth",
				workflowPlaneKindLabel: workflowPlaneKindWorkflowPlane,
				workflowPlaneNameLabel: "wp-default",
			},
		},
	}

	nonGitSecretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				"openchoreo.dev/secret-type": "other",
			},
		},
	}

	// SecretReference without workflow plane labels (legacy)
	legacyGitSecretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel: gitSecretTypeValue,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(gitSecretRef, nonGitSecretRef, legacyGitSecretRef).
		Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	secrets, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(secrets) != 2 {
		t.Fatalf("expected 2 git secrets, got %d", len(secrets))
	}

	// Find the secret with workflow plane labels
	var found bool
	for _, s := range secrets {
		if s.Name == "git-secret-1" {
			found = true
			if s.WorkflowPlaneKind != workflowPlaneKindWorkflowPlane {
				t.Errorf("WorkflowPlaneKind = %q, want %q", s.WorkflowPlaneKind, workflowPlaneKindWorkflowPlane)
			}
			if s.WorkflowPlaneName != "wp-default" {
				t.Errorf("WorkflowPlaneName = %q, want %q", s.WorkflowPlaneName, "wp-default")
			}
		}
		if s.Name == "legacy-secret" {
			if s.WorkflowPlaneKind != "" {
				t.Errorf("legacy secret WorkflowPlaneKind = %q, want empty", s.WorkflowPlaneKind)
			}
			if s.WorkflowPlaneName != "" {
				t.Errorf("legacy secret WorkflowPlaneName = %q, want empty", s.WorkflowPlaneName)
			}
		}
	}
	if !found {
		t.Error("git-secret-1 not found in results")
	}
}

func TestListGitSecrets_EmptyNamespace(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	secrets, err := svc.ListGitSecrets(context.Background(), "empty-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(secrets))
	}
}

// --- resolveWorkflowPlane tests ---

func TestResolveWorkflowPlane_NamespacedWorkflowPlane(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-secret-store",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	// PlaneClientProvider is nil here — only error paths (not-found, no-secret-store) are exercised.
	// For full plane-client tests, use newTestServiceWithWPClient which injects a mock provider.

	// Test not found
	_, err := svc.resolveWorkflowPlane(context.Background(), testNamespace, workflowPlaneKindWorkflowPlane, "nonexistent")
	if !errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Errorf("expected ErrWorkflowPlaneNotFound, got %v", err)
	}
}

func TestResolveWorkflowPlane_ClusterWorkflowPlane(t *testing.T) {
	scheme := newTestScheme(t)
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cwp-shared",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "cluster-secret-store",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cwp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	// Test not found
	_, err := svc.resolveWorkflowPlane(context.Background(), testNamespace, workflowPlaneKindClusterWorkflowPlane, "nonexistent")
	if !errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Errorf("expected ErrWorkflowPlaneNotFound, got %v", err)
	}
}

func TestResolveWorkflowPlane_NoSecretStore(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-no-store",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.resolveNamespacedWorkflowPlane(context.Background(), testNamespace, "wp-no-store")
	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

func TestResolveWorkflowPlane_ClusterNoSecretStore(t *testing.T) {
	scheme := newTestScheme(t)
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cwp-no-store",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      "no-store",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cwp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.resolveClusterWorkflowPlane(context.Background(), "cwp-no-store")
	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

func TestResolveWorkflowPlane_InvalidKind(t *testing.T) {
	svc := &gitSecretService{logger: newTestLogger()}

	_, err := svc.resolveWorkflowPlane(context.Background(), testNamespace, "InvalidKind", "name")
	if err == nil {
		t.Fatal("expected error for invalid kind, got nil")
	}
	var valErr *services.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

// --- DeleteGitSecret tests ---

func TestDeleteGitSecret_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "nonexistent")
	if !errors.Is(err, ErrGitSecretNotFound) {
		t.Errorf("expected ErrGitSecretNotFound, got %v", err)
	}
}

func TestDeleteGitSecret_NotGitCredentials(t *testing.T) {
	scheme := newTestScheme(t)
	nonGitRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				"openchoreo.dev/secret-type": "other",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nonGitRef).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "other-secret")
	if !errors.Is(err, ErrGitSecretNotFound) {
		t.Errorf("expected ErrGitSecretNotFound, got %v", err)
	}
}

func TestDeleteGitSecret_MissingWorkflowPlaneLabels(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel: gitSecretTypeValue,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "legacy-secret")
	if err == nil {
		t.Fatal("expected error for missing workflow plane labels, got nil")
	}
}

// --- CreateGitSecret tests ---

func TestCreateGitSecret_AlreadyExists(t *testing.T) {
	scheme := newTestScheme(t)
	existing := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-secret",
			Namespace: testNamespace,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "existing-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})

	if !errors.Is(err, ErrGitSecretAlreadyExists) {
		t.Errorf("expected ErrGitSecretAlreadyExists, got %v", err)
	}
}

func TestCreateGitSecret_ValidationError(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "", // missing required token
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var valErr *services.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreateGitSecret_WorkflowPlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "nonexistent",
	})

	if !errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Errorf("expected ErrWorkflowPlaneNotFound, got %v", err)
	}
}

// --- getWorkflowNamespace tests ---

func TestGetWorkflowNamespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{testNamespace, "workflows-ns1"},
		{"my-namespace", "workflows-my-namespace"},
	}

	for _, tt := range tests {
		got := getWorkflowNamespace(tt.input)
		if got != tt.want {
			t.Errorf("getWorkflowNamespace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- createPushSecret tests ---

func TestCreatePushSecret(t *testing.T) {
	svc := &gitSecretService{logger: newTestLogger()}

	type expectedDataMatch struct {
		secretKey string
		property  string
	}

	const wantRemoteKey = "secret/ns1/git/test-secret"

	tests := []struct {
		name         string
		secretType   string
		username     string
		sshKeyID     string
		expectedData []expectedDataMatch
	}{
		{
			name:       "basic-auth with username",
			secretType: "basic-auth",
			username:   "user1",
			expectedData: []expectedDataMatch{
				{secretKey: "password", property: "password"},
				{secretKey: "username", property: "username"},
			},
		},
		{
			name:       "basic-auth without username",
			secretType: "basic-auth",
			expectedData: []expectedDataMatch{
				{secretKey: "password", property: "password"},
			},
		},
		{
			name:       "ssh-auth with key ID",
			secretType: "ssh-auth",
			sshKeyID:   "key-id",
			expectedData: []expectedDataMatch{
				{secretKey: "ssh-privatekey", property: "ssh-privatekey"},
				{secretKey: "ssh-key-id", property: "ssh-key-id"},
			},
		},
		{
			name:       "ssh-auth without key ID",
			secretType: "ssh-auth",
			expectedData: []expectedDataMatch{
				{secretKey: "ssh-privatekey", property: "ssh-privatekey"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := svc.createPushSecret("test-secret", "my-store", testNamespace, "workflows-ns1", tt.secretType, tt.username, tt.sshKeyID)

			if ps.GetAPIVersion() != "external-secrets.io/v1alpha1" {
				t.Errorf("APIVersion = %q, want %q", ps.GetAPIVersion(), "external-secrets.io/v1alpha1")
			}
			if ps.GetKind() != "PushSecret" {
				t.Errorf("Kind = %q, want %q", ps.GetKind(), "PushSecret")
			}
			if ps.GetName() != "test-secret" {
				t.Errorf("Name = %q, want %q", ps.GetName(), "test-secret")
			}
			if ps.GetNamespace() != "workflows-ns1" {
				t.Errorf("Namespace = %q, want %q", ps.GetNamespace(), "workflows-ns1")
			}
			if ps.GetLabels()[ownerNamespaceLabel] != testNamespace {
				t.Errorf("owner label = %q, want %q", ps.GetLabels()[ownerNamespaceLabel], testNamespace)
			}

			spec, ok := ps.Object["spec"].(map[string]any)
			if !ok {
				t.Fatal("spec is not a map")
			}
			if spec["updatePolicy"] != "Replace" {
				t.Errorf("updatePolicy = %v, want %q", spec["updatePolicy"], "Replace")
			}

			storeRefs, ok := spec["secretStoreRefs"].([]map[string]any)
			if !ok || len(storeRefs) != 1 {
				t.Fatal("expected 1 secretStoreRef")
			}
			if storeRefs[0]["name"] != "my-store" {
				t.Errorf("secretStoreRef name = %v, want %q", storeRefs[0]["name"], "my-store")
			}

			data, ok := spec["data"].([]map[string]any)
			if !ok {
				t.Fatal("data is not a slice of maps")
			}
			if len(data) != len(tt.expectedData) {
				t.Fatalf("data matches = %d, want %d", len(data), len(tt.expectedData))
			}
			for i, want := range tt.expectedData {
				match, ok := data[i]["match"].(map[string]any)
				if !ok {
					t.Fatalf("data[%d].match is not a map", i)
				}
				if match["secretKey"] != want.secretKey {
					t.Errorf("data[%d].match.secretKey = %v, want %q", i, match["secretKey"], want.secretKey)
				}
				remoteRef, ok := match["remoteRef"].(map[string]any)
				if !ok {
					t.Fatalf("data[%d].match.remoteRef is not a map", i)
				}
				if remoteRef["remoteKey"] != wantRemoteKey {
					t.Errorf("data[%d].match.remoteRef.remoteKey = %v, want %q", i, remoteRef["remoteKey"], wantRemoteKey)
				}
				if remoteRef["property"] != want.property {
					t.Errorf("data[%d].match.remoteRef.property = %v, want %q", i, remoteRef["property"], want.property)
				}
			}
		})
	}
}

// --- ensureNamespaceExists tests ---

func TestEnsureNamespaceExists_AlreadyExists(t *testing.T) {
	scheme := newTestScheme(t)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "existing-ns",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	svc := &gitSecretService{logger: newTestLogger()}

	err := svc.ensureNamespaceExists(context.Background(), k8sClient, "existing-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNamespaceExists_CreatesNew(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &gitSecretService{logger: newTestLogger()}

	err := svc.ensureNamespaceExists(context.Background(), k8sClient, "new-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the namespace was created
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "new-ns"}, ns); err != nil {
		t.Fatalf("namespace was not created: %v", err)
	}
}

// --- NewService test ---

func TestNewService(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := newTestLogger()

	mockProvider := &k8sMocks.MockWorkflowPlaneClientProvider{}
	svc := NewService(k8sClient, mockProvider, logger)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// --- DeleteGitSecret additional tests ---

func TestDeleteGitSecret_WorkflowPlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				workflowPlaneKindLabel: workflowPlaneKindWorkflowPlane,
				workflowPlaneNameLabel: "nonexistent-wp",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if !errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Errorf("expected ErrWorkflowPlaneNotFound, got %v", err)
	}
}

func TestDeleteGitSecret_InvalidWorkflowPlaneKind(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				workflowPlaneKindLabel: "InvalidKind",
				workflowPlaneNameLabel: "some-name",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var valErr *services.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

// --- CreateGitSecret additional tests ---

func TestCreateGitSecret_UnsupportedSecretType(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "unsupported",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var valErr *services.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreateGitSecret_InvalidWorkflowPlaneKind(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: "InvalidKind",
		WorkflowPlaneName: "name",
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var valErr *services.ValidationError
	if !isValidationError(err, &valErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreateGitSecret_SSHAuthWorkflowPlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "ssh-secret",
		SecretType:        "ssh-auth",
		SSHKey:            "-----BEGIN KEY-----",
		SSHKeyID:          "key-id",
		WorkflowPlaneKind: workflowPlaneKindClusterWorkflowPlane,
		WorkflowPlaneName: "nonexistent",
	})

	if !errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Errorf("expected ErrWorkflowPlaneNotFound, got %v", err)
	}
}

func TestCreateGitSecret_SecretStoreNotConfigured(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-no-store",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-no-store",
	})

	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

// --- resolveWorkflowPlane additional tests ---

// --- ListGitSecrets error tests ---

func TestListGitSecrets_ClientError(t *testing.T) {
	scheme := newTestScheme(t)
	listErr := errors.New("connection refused")
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return listErr
			},
		}).
		Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.ListGitSecrets(context.Background(), testNamespace)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- ensureNamespaceExists error tests ---

func TestEnsureNamespaceExists_GetError(t *testing.T) {
	scheme := newTestScheme(t)
	getErr := errors.New("api server unavailable")
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return getErr
			},
		}).
		Build()

	svc := &gitSecretService{logger: newTestLogger()}

	err := svc.ensureNamespaceExists(context.Background(), k8sClient, "some-ns")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEnsureNamespaceExists_CreateError(t *testing.T) {
	scheme := newTestScheme(t)
	createErr := errors.New("quota exceeded")
	callCount := 0
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, key client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				// Simulate namespace not found
				return apierrors.NewNotFound(corev1.Resource("namespaces"), key.Name)
			},
			Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
				callCount++
				return createErr
			},
		}).
		Build()

	svc := &gitSecretService{logger: newTestLogger()}

	err := svc.ensureNamespaceExists(context.Background(), k8sClient, "new-ns")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("expected Create to be called once, got %d", callCount)
	}
}

func TestEnsureNamespaceExists_ConcurrentCreation(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, key client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewNotFound(corev1.Resource("namespaces"), key.Name)
			},
			Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
				return apierrors.NewAlreadyExists(corev1.Resource("namespaces"), "new-ns")
			},
		}).
		Build()

	svc := &gitSecretService{logger: newTestLogger()}

	err := svc.ensureNamespaceExists(context.Background(), k8sClient, "new-ns")
	if err != nil {
		t.Fatalf("expected no error on concurrent creation, got %v", err)
	}
}

func TestResolveWorkflowPlane_ClusterSecretStoreEmptyName(t *testing.T) {
	scheme := newTestScheme(t)
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cwp-empty-store",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      "empty-store",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cwp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.resolveClusterWorkflowPlane(context.Background(), "cwp-empty-store")
	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

// newTestServiceWithWPClient creates a gitSecretService with a mock PlaneClientProvider
// that returns the given fake WP client for any workflow plane.
func newTestServiceWithWPClient(t *testing.T, cpObjects []client.Object, wpClient client.Client) *gitSecretService {
	t.Helper()
	scheme := newTestScheme(t)
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cpObjects...).Build()

	mockProvider := &k8sMocks.MockWorkflowPlaneClientProvider{}
	mockProvider.EXPECT().WorkflowPlaneClient(mock.Anything).Return(wpClient, nil).Maybe()
	mockProvider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(wpClient, nil).Maybe()

	return &gitSecretService{
		k8sClient:           cpClient,
		planeClientProvider: mockProvider,
		logger:              newTestLogger(),
	}
}

// --- resolveWorkflowPlane happy path tests ---

func TestResolveNamespacedWorkflowPlane_Success(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-secret-store",
			},
		},
	}
	wpFakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := newTestServiceWithWPClient(t, []client.Object{wp}, wpFakeClient)

	info, err := svc.resolveNamespacedWorkflowPlane(context.Background(), testNamespace, "wp-default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.secretStoreName != "my-secret-store" {
		t.Errorf("secretStoreName = %q, want %q", info.secretStoreName, "my-secret-store")
	}
	if info.client == nil {
		t.Error("expected non-nil client")
	}
}

func TestResolveClusterWorkflowPlane_Success(t *testing.T) {
	scheme := newTestScheme(t)
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cwp-shared",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      "cwp-shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "cluster-secret-store",
			},
		},
	}
	wpFakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := newTestServiceWithWPClient(t, []client.Object{cwp}, wpFakeClient)

	info, err := svc.resolveClusterWorkflowPlane(context.Background(), "cwp-shared")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.secretStoreName != "cluster-secret-store" {
		t.Errorf("secretStoreName = %q, want %q", info.secretStoreName, "cluster-secret-store")
	}
	if info.client == nil {
		t.Error("expected non-nil client")
	}
}

// --- CreateGitSecret happy path test ---

func TestCreateGitSecret_Success_BasicAuth(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-secret-store",
			},
		},
	}

	// Use interceptor to handle SSA Patch calls on the WP client
	patchCount := 0
	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				patchCount++
				return nil
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{wp}, wpFakeClient)

	result, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Username:          "user1",
		Token:             "ghp_token123",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "new-secret" {
		t.Errorf("result Name = %q, want %q", result.Name, "new-secret")
	}
	if result.Namespace != testNamespace {
		t.Errorf("result Namespace = %q, want %q", result.Namespace, testNamespace)
	}
	if result.WorkflowPlaneKind != workflowPlaneKindWorkflowPlane {
		t.Errorf("result WorkflowPlaneKind = %q, want %q", result.WorkflowPlaneKind, workflowPlaneKindWorkflowPlane)
	}
	if patchCount != 2 {
		t.Errorf("expected 2 Patch calls (secret + pushsecret), got %d", patchCount)
	}
}

func TestCreateGitSecret_Success_SSHAuth(t *testing.T) {
	scheme := newTestScheme(t)
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cwp-shared",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      "cwp-shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "cluster-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return nil
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{cwp}, wpFakeClient)

	result, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "ssh-secret",
		SecretType:        "ssh-auth",
		SSHKey:            "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----",
		SSHKeyID:          "key-id-123",
		WorkflowPlaneKind: workflowPlaneKindClusterWorkflowPlane,
		WorkflowPlaneName: "cwp-shared",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "ssh-secret" {
		t.Errorf("result Name = %q, want %q", result.Name, "ssh-secret")
	}
	if result.WorkflowPlaneKind != workflowPlaneKindClusterWorkflowPlane {
		t.Errorf("result WorkflowPlaneKind = %q, want %q", result.WorkflowPlaneKind, workflowPlaneKindClusterWorkflowPlane)
	}
}

// --- CreateGitSecret error after WP resolution ---

func TestCreateGitSecret_WPSecretPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return errors.New("patch failed")
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{wp}, wpFakeClient)

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateGitSecret_PushSecretPatchError(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	patchCount := 0
	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				patchCount++
				if patchCount == 2 {
					return errors.New("push secret patch failed")
				}
				return nil
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{wp}, wpFakeClient)

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateGitSecret_SecretRefCreateError(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return nil
			},
		}).
		Build()

	// Use an interceptor on the CP client to make SecretReference creation fail
	cpClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(wp).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
				return errors.New("cp create failed")
			},
		}).
		Build()

	mockProvider := &k8sMocks.MockWorkflowPlaneClientProvider{}
	mockProvider.EXPECT().WorkflowPlaneClient(mock.Anything).Return(wpFakeClient, nil).Maybe()
	mockProvider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(wpFakeClient, nil).Maybe()

	svc := &gitSecretService{
		k8sClient:           cpClient,
		planeClientProvider: mockProvider,
		logger:              newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- DeleteGitSecret happy path test ---

func TestDeleteGitSecret_Success(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				workflowPlaneKindLabel: workflowPlaneKindWorkflowPlane,
				workflowPlaneNameLabel: "wp-default",
			},
		},
	}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := newTestServiceWithWPClient(t, []client.Object{secretRef, wp}, wpFakeClient)

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the SecretReference was deleted
	deleted := &openchoreov1alpha1.SecretReference{}
	getErr := svc.k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-secret", Namespace: testNamespace}, deleted)
	if getErr == nil {
		t.Error("expected SecretReference to be deleted")
	}
}

func TestDeleteGitSecret_WPPushSecretDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				workflowPlaneKindLabel: workflowPlaneKindWorkflowPlane,
				workflowPlaneNameLabel: "wp-default",
			},
		},
	}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return errors.New("delete failed")
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{secretRef, wp}, wpFakeClient)

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteGitSecret_GetError(t *testing.T) {
	scheme := newTestScheme(t)
	cpClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return errors.New("api server error")
			},
		}).
		Build()

	svc := &gitSecretService{
		k8sClient: cpClient,
		logger:    newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveWorkflowPlane_NamespacedSecretStoreEmptyName(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-empty-store",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.resolveNamespacedWorkflowPlane(context.Background(), testNamespace, "wp-empty-store")
	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

func TestResolveNamespacedWorkflowPlane_ClientError(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()
	// Mock provider returns error to simulate client creation failure
	mockProvider := &k8sMocks.MockWorkflowPlaneClientProvider{}
	mockProvider.EXPECT().WorkflowPlaneClient(mock.Anything).Return(nil, fmt.Errorf("client creation failed")).Maybe()
	svc := &gitSecretService{
		k8sClient:           k8sClient,
		planeClientProvider: mockProvider,
		logger:              newTestLogger(),
	}

	_, err := svc.resolveNamespacedWorkflowPlane(context.Background(), testNamespace, "wp-default")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveClusterWorkflowPlane_ClientError(t *testing.T) {
	scheme := newTestScheme(t)
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cwp-shared",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID:      "cwp-shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "cluster-store",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cwp).Build()
	// Mock provider returns error to simulate client creation failure
	mockProvider := &k8sMocks.MockWorkflowPlaneClientProvider{}
	mockProvider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(nil, fmt.Errorf("client creation failed")).Maybe()
	svc := &gitSecretService{
		k8sClient:           k8sClient,
		planeClientProvider: mockProvider,
		logger:              newTestLogger(),
	}

	_, err := svc.resolveClusterWorkflowPlane(context.Background(), "cwp-shared")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteGitSecret_WPSecretDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				workflowPlaneKindLabel: workflowPlaneKindWorkflowPlane,
				workflowPlaneNameLabel: "wp-default",
			},
		},
	}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	deleteCount := 0
	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				deleteCount++
				// First delete (PushSecret) succeeds with NotFound (ignored)
				// Second delete (K8s Secret) fails
				if deleteCount == 2 {
					return errors.New("secret delete error")
				}
				return apierrors.NewNotFound(corev1.Resource("pushsecrets"), "test-secret")
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{secretRef, wp}, wpFakeClient)

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteGitSecret_CPSecretRefDeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: testNamespace,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				workflowPlaneKindLabel: workflowPlaneKindWorkflowPlane,
				workflowPlaneNameLabel: "wp-default",
			},
		},
	}
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// CP client: Get succeeds normally, Delete fails
	cpClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secretRef, wp).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return errors.New("cp delete failed")
			},
		}).
		Build()

	mockProvider := &k8sMocks.MockWorkflowPlaneClientProvider{}
	mockProvider.EXPECT().WorkflowPlaneClient(mock.Anything).Return(wpFakeClient, nil).Maybe()
	mockProvider.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(wpFakeClient, nil).Maybe()

	svc := &gitSecretService{
		k8sClient:           cpClient,
		planeClientProvider: mockProvider,
		logger:              newTestLogger(),
	}

	err := svc.DeleteGitSecret(context.Background(), testNamespace, "test-secret")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveNamespacedWorkflowPlane_GetNonNotFoundError(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return errors.New("api server timeout")
			},
		}).
		Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.resolveNamespacedWorkflowPlane(context.Background(), testNamespace, "wp-default")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Error("should not be ErrWorkflowPlaneNotFound for non-NotFound errors")
	}
}

func TestResolveClusterWorkflowPlane_GetNonNotFoundError(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return errors.New("api server timeout")
			},
		}).
		Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.resolveClusterWorkflowPlane(context.Background(), "cwp-shared")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Error("should not be ErrWorkflowPlaneNotFound for non-NotFound errors")
	}
}

func TestCreateGitSecret_GetExistingCheckError(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return errors.New("api server unavailable")
			},
		}).
		Build()

	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateGitSecret_EnsureNamespaceError(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-default",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{
				Name: "my-store",
			},
		},
	}

	wpFakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return errors.New("wp get failed")
			},
		}).
		Build()

	svc := newTestServiceWithWPClient(t, []client.Object{wp}, wpFakeClient)

	_, err := svc.CreateGitSecret(context.Background(), testNamespace, &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: workflowPlaneKindWorkflowPlane,
		WorkflowPlaneName: "wp-default",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
