// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
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
			namespace:  "ns1",
			secretName: "my-secret",
			secretType: "basic-auth",
			username:   "user1",
			wpKind:     "WorkflowPlane",
			wpName:     "wp-default",
		},
		{
			name:       "ssh-auth with ClusterWorkflowPlane",
			namespace:  "ns2",
			secretName: "ssh-secret",
			secretType: "ssh-auth",
			sshKeyID:   "key-id-123",
			wpKind:     "ClusterWorkflowPlane",
			wpName:     "cwp-shared",
		},
		{
			name:       "basic-auth without username",
			namespace:  "ns1",
			secretName: "token-only",
			secretType: "basic-auth",
			wpKind:     "WorkflowPlane",
			wpName:     "wp1",
		},
		{
			name:       "ssh-auth without key ID",
			namespace:  "ns1",
			secretName: "ssh-only",
			secretType: "ssh-auth",
			wpKind:     "ClusterWorkflowPlane",
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
			secret := svc.buildGitSecret("test-secret", "ns1", "workflows-ns1", tt.secretType, tt.username, tt.token, tt.sshKey, tt.sshKeyID)

			if secret.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", secret.Type, tt.wantType)
			}
			if secret.Labels[ownerNamespaceLabel] != "ns1" {
				t.Errorf("owner namespace label = %q, want %q", secret.Labels[ownerNamespaceLabel], "ns1")
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
			Namespace: "ns1",
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				gitSecretAuthTypeLabel: "basic-auth",
				workflowPlaneKindLabel: "WorkflowPlane",
				workflowPlaneNameLabel: "wp-default",
			},
		},
	}

	nonGitSecretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-secret",
			Namespace: "ns1",
			Labels: map[string]string{
				"openchoreo.dev/secret-type": "other",
			},
		},
	}

	// SecretReference without workflow plane labels (legacy)
	legacyGitSecretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: "ns1",
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

	secrets, err := svc.ListGitSecrets(context.Background(), "ns1")
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
			if s.WorkflowPlaneKind != "WorkflowPlane" {
				t.Errorf("WorkflowPlaneKind = %q, want %q", s.WorkflowPlaneKind, "WorkflowPlane")
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
			Namespace: "ns1",
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

	// Cannot fully test because GetK8sClientFromWorkflowPlane requires a real KubeMultiClientManager.
	// But we can verify error paths.

	// Test not found
	_, err := svc.resolveWorkflowPlane(context.Background(), "ns1", "WorkflowPlane", "nonexistent")
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
	_, err := svc.resolveWorkflowPlane(context.Background(), "ns1", "ClusterWorkflowPlane", "nonexistent")
	if !errors.Is(err, ErrWorkflowPlaneNotFound) {
		t.Errorf("expected ErrWorkflowPlaneNotFound, got %v", err)
	}
}

func TestResolveWorkflowPlane_NoSecretStore(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wp-no-store",
			Namespace: "ns1",
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

	_, err := svc.resolveNamespacedWorkflowPlane(context.Background(), "ns1", "wp-no-store")
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

	_, err := svc.resolveWorkflowPlane(context.Background(), "ns1", "InvalidKind", "name")
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

	err := svc.DeleteGitSecret(context.Background(), "ns1", "nonexistent")
	if !errors.Is(err, ErrGitSecretNotFound) {
		t.Errorf("expected ErrGitSecretNotFound, got %v", err)
	}
}

func TestDeleteGitSecret_NotGitCredentials(t *testing.T) {
	scheme := newTestScheme(t)
	nonGitRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-secret",
			Namespace: "ns1",
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

	err := svc.DeleteGitSecret(context.Background(), "ns1", "other-secret")
	if !errors.Is(err, ErrGitSecretNotFound) {
		t.Errorf("expected ErrGitSecretNotFound, got %v", err)
	}
}

func TestDeleteGitSecret_MissingWorkflowPlaneLabels(t *testing.T) {
	scheme := newTestScheme(t)
	secretRef := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-secret",
			Namespace: "ns1",
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

	err := svc.DeleteGitSecret(context.Background(), "ns1", "legacy-secret")
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
			Namespace: "ns1",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	svc := &gitSecretService{
		k8sClient: k8sClient,
		logger:    newTestLogger(),
	}

	_, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
		SecretName:        "existing-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: "WorkflowPlane",
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

	_, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "", // missing required token
		WorkflowPlaneKind: "WorkflowPlane",
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

	_, err := svc.CreateGitSecret(context.Background(), "ns1", &CreateGitSecretParams{
		SecretName:        "new-secret",
		SecretType:        "basic-auth",
		Token:             "token",
		WorkflowPlaneKind: "WorkflowPlane",
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
		{"ns1", "workflows-ns1"},
		{"my-namespace", "workflows-my-namespace"},
	}

	for _, tt := range tests {
		got := getWorkflowNamespace(tt.input)
		if got != tt.want {
			t.Errorf("getWorkflowNamespace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
