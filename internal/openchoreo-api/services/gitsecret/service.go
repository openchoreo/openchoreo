// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitsecret

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

//nolint:gosec // False positive: these are annotation keys and namespace prefixes, not credentials
const (
	secretTypeBasicAuth      = "basic-auth"
	secretTypeSSHAuth        = "ssh-auth"
	gitSecretTypeLabel       = "openchoreo.dev/secret-type"
	gitSecretTypeValue       = "git-credentials"
	gitSecretAuthTypeLabel   = "kubernetes.io/secret-type"
	ownerNamespaceLabel      = "openchoreo.dev/owner-namespace"
	workflowPlaneKindLabel   = "openchoreo.dev/workflow-plane-kind"
	workflowPlaneNameLabel   = "openchoreo.dev/workflow-plane-name"
	gitSecretNamespacePrefix = "workflows-"

	workflowPlaneKindWorkflowPlane        = "WorkflowPlane"
	workflowPlaneKindClusterWorkflowPlane = "ClusterWorkflowPlane"
)

// getWorkflowNamespace returns the workflow execution namespace for a given control plane namespace.
func getWorkflowNamespace(namespaceName string) string {
	return gitSecretNamespacePrefix + namespaceName
}

type gitSecretService struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.WorkflowPlaneClientProvider
	logger              *slog.Logger
}

// NewService creates a new git secret service without authorization.
func NewService(k8sClient client.Client, planeClientProvider kubernetesClient.WorkflowPlaneClientProvider, logger *slog.Logger) Service {
	return &gitSecretService{
		k8sClient:           k8sClient,
		planeClientProvider: planeClientProvider,
		logger:              logger,
	}
}

// ListGitSecrets returns all git secrets in a namespace.
func (s *gitSecretService) ListGitSecrets(ctx context.Context, namespaceName string) ([]GitSecretInfo, error) {
	s.logger.Debug("Listing git secrets", "namespace", namespaceName)

	var secretRefs openchoreov1alpha1.SecretReferenceList
	if err := s.k8sClient.List(ctx, &secretRefs, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list secret references", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list git secrets: %w", err)
	}

	secrets := make([]GitSecretInfo, 0, len(secretRefs.Items))
	for _, ref := range secretRefs.Items {
		if ref.Labels[gitSecretTypeLabel] == gitSecretTypeValue {
			secrets = append(secrets, GitSecretInfo{
				Name:              ref.Name,
				Namespace:         ref.Namespace,
				WorkflowPlaneKind: ref.Labels[workflowPlaneKindLabel],
				WorkflowPlaneName: ref.Labels[workflowPlaneNameLabel],
			})
		}
	}

	return secrets, nil
}

// CreateGitSecret creates a git secret across control and workflow planes.
func (s *gitSecretService) CreateGitSecret(ctx context.Context, namespaceName string, req *CreateGitSecretParams) (*GitSecretInfo, error) {
	s.logger.Debug("Creating git secret", "namespace", namespaceName, "secret", req.SecretName, "type", req.SecretType)

	if err := validateCredentials(req); err != nil {
		return nil, err
	}

	// Check if SecretReference already exists in control plane
	existingSecretRef := &openchoreov1alpha1.SecretReference{}
	secretRefKey := client.ObjectKey{Name: req.SecretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, secretRefKey, existingSecretRef); err == nil {
		s.logger.Warn("Git secret already exists", "namespace", namespaceName, "secret", req.SecretName)
		return nil, ErrGitSecretAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		s.logger.Error("Failed to check existing secret reference", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to check existing secret reference: %w", err)
	}

	wpInfo, err := s.resolveWorkflowPlane(ctx, namespaceName, req.WorkflowPlaneKind, req.WorkflowPlaneName)
	if err != nil {
		return nil, err
	}

	workflowNamespace := getWorkflowNamespace(namespaceName)
	if err := s.ensureNamespaceExists(ctx, wpInfo.client, workflowNamespace); err != nil {
		return nil, err
	}

	// Create or update K8s Secret in workflow plane using Server-Side Apply
	secret := s.buildGitSecret(req.SecretName, namespaceName, workflowNamespace, req.SecretType, req.Username, req.Token, req.SSHKey, req.SSHKeyID)
	if err := wpInfo.client.Patch(ctx, secret, client.Apply, client.ForceOwnership, client.FieldOwner("openchoreo-api")); err != nil {
		s.logger.Error("Failed to apply workflow plane secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to apply workflow plane secret: %w", err)
	}
	s.logger.Debug("Successfully applied K8s secret in workflow plane", "namespace", workflowNamespace, "secret", req.SecretName)

	// Create or update PushSecret in workflow plane using Server-Side Apply
	pushSecret := s.createPushSecret(req.SecretName, wpInfo.secretStoreName, namespaceName, workflowNamespace, req.SecretType, req.Username, req.SSHKeyID)
	if err := wpInfo.client.Patch(ctx, pushSecret, client.Apply, client.ForceOwnership, client.FieldOwner("openchoreo-api")); err != nil {
		s.logger.Error("Failed to apply push secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to apply push secret: %w", err)
	}
	s.logger.Debug("Successfully applied PushSecret in workflow plane", "namespace", workflowNamespace, "secret", req.SecretName)

	// Create SecretReference in control plane
	secretReference := s.buildSecretReference(namespaceName, req.SecretName, req.SecretType, req.Username, req.SSHKeyID, req.WorkflowPlaneKind, req.WorkflowPlaneName)
	if err := s.k8sClient.Create(ctx, secretReference); err != nil {
		s.logger.Error("Failed to create secret reference", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to create secret reference: %w", err)
	}

	s.logger.Info("Successfully created git secret", "namespace", namespaceName, "secret", req.SecretName, "type", req.SecretType)
	return &GitSecretInfo{
		Name:              req.SecretName,
		Namespace:         namespaceName,
		WorkflowPlaneKind: req.WorkflowPlaneKind,
		WorkflowPlaneName: req.WorkflowPlaneName,
	}, nil
}

// DeleteGitSecret deletes a git secret from control and workflow planes.
func (s *gitSecretService) DeleteGitSecret(ctx context.Context, namespaceName, secretName string) error {
	s.logger.Debug("Deleting git secret", "namespace", namespaceName, "secret", secretName)

	secretRef := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{Name: secretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, secretRef); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrGitSecretNotFound
		}
		s.logger.Error("Failed to get secret reference", "error", err, "namespace", namespaceName, "secret", secretName)
		return fmt.Errorf("failed to get secret reference: %w", err)
	}

	if secretRef.Labels[gitSecretTypeLabel] != gitSecretTypeValue {
		return ErrGitSecretNotFound
	}

	wpKind := secretRef.Labels[workflowPlaneKindLabel]
	wpName := secretRef.Labels[workflowPlaneNameLabel]
	if wpKind == "" || wpName == "" {
		s.logger.Error("SecretReference missing workflow plane labels", "namespace", namespaceName, "secret", secretName)
		return fmt.Errorf("secret reference is missing workflow plane labels")
	}

	wpInfo, err := s.resolveWorkflowPlane(ctx, namespaceName, wpKind, wpName)
	if err != nil {
		return err
	}

	workflowNamespace := getWorkflowNamespace(namespaceName)

	// Delete PushSecret from workflow plane
	pushSecret := &unstructured.Unstructured{}
	pushSecret.SetAPIVersion("external-secrets.io/v1alpha1")
	pushSecret.SetKind("PushSecret")
	pushSecret.SetName(secretName)
	pushSecret.SetNamespace(workflowNamespace)
	if err := wpInfo.client.Delete(ctx, pushSecret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			s.logger.Error("Failed to delete push secret", "error", err, "namespace", namespaceName, "secret", secretName)
			return fmt.Errorf("failed to delete push secret: %w", err)
		}
		s.logger.Debug("Push secret not found, skipping", "namespace", namespaceName, "secret", secretName)
	}

	// Delete Kubernetes Secret from workflow plane
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: workflowNamespace,
		},
	}
	if err := wpInfo.client.Delete(ctx, secret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			s.logger.Error("Failed to delete workflow plane secret", "error", err, "namespace", namespaceName, "secret", secretName)
			return fmt.Errorf("failed to delete workflow plane secret: %w", err)
		}
		s.logger.Debug("Workflow plane secret not found, skipping", "namespace", namespaceName, "secret", secretName)
	}

	// Delete SecretReference CRD from control plane
	if err := s.k8sClient.Delete(ctx, secretRef); err != nil {
		if client.IgnoreNotFound(err) != nil {
			s.logger.Error("Failed to delete secret reference", "error", err, "namespace", namespaceName, "secret", secretName)
			return fmt.Errorf("failed to delete secret reference: %w", err)
		}
	}

	s.logger.Info("Successfully deleted git secret", "namespace", namespaceName, "secret", secretName)
	return nil
}

// workflowPlaneInfo holds the resolved workflow plane details needed by the service.
type workflowPlaneInfo struct {
	client          client.Client
	secretStoreName string
}

// resolveWorkflowPlane fetches the workflow plane by kind and name, validates it, and returns a client.
func (s *gitSecretService) resolveWorkflowPlane(ctx context.Context, namespaceName, kind, name string) (*workflowPlaneInfo, error) {
	switch kind {
	case workflowPlaneKindWorkflowPlane:
		return s.resolveNamespacedWorkflowPlane(ctx, namespaceName, name)
	case workflowPlaneKindClusterWorkflowPlane:
		return s.resolveClusterWorkflowPlane(ctx, name)
	default:
		return nil, &services.ValidationError{Msg: fmt.Sprintf("unsupported workflow plane kind: %s", kind)}
	}
}

func (s *gitSecretService) resolveNamespacedWorkflowPlane(ctx context.Context, namespaceName, name string) (*workflowPlaneInfo, error) {
	wp := &openchoreov1alpha1.WorkflowPlane{}
	key := client.ObjectKey{Name: name, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, wp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("WorkflowPlane not found", "namespace", namespaceName, "name", name)
			return nil, ErrWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to get WorkflowPlane", "error", err, "namespace", namespaceName, "name", name)
		return nil, fmt.Errorf("failed to get workflow plane: %w", err)
	}

	if wp.Spec.SecretStoreRef == nil || wp.Spec.SecretStoreRef.Name == "" {
		s.logger.Warn("WorkflowPlane has no secret store configured", "namespace", namespaceName, "name", name)
		return nil, ErrSecretStoreNotConfigured
	}

	wpClient, err := s.planeClientProvider.WorkflowPlaneClient(wp)
	if err != nil {
		s.logger.Error("Failed to get workflow plane client", "error", err, "namespace", namespaceName, "name", name)
		return nil, fmt.Errorf("failed to get workflow plane client: %w", err)
	}

	return &workflowPlaneInfo{
		client:          wpClient,
		secretStoreName: wp.Spec.SecretStoreRef.Name,
	}, nil
}

func (s *gitSecretService) resolveClusterWorkflowPlane(ctx context.Context, name string) (*workflowPlaneInfo, error) {
	cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
	key := client.ObjectKey{Name: name}
	if err := s.k8sClient.Get(ctx, key, cwp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("ClusterWorkflowPlane not found", "name", name)
			return nil, ErrWorkflowPlaneNotFound
		}
		s.logger.Error("Failed to get ClusterWorkflowPlane", "error", err, "name", name)
		return nil, fmt.Errorf("failed to get cluster workflow plane: %w", err)
	}

	if cwp.Spec.SecretStoreRef == nil || cwp.Spec.SecretStoreRef.Name == "" {
		s.logger.Warn("ClusterWorkflowPlane has no secret store configured", "name", name)
		return nil, ErrSecretStoreNotConfigured
	}

	wpClient, err := s.planeClientProvider.ClusterWorkflowPlaneClient(cwp)
	if err != nil {
		s.logger.Error("Failed to get cluster workflow plane client", "error", err, "name", name)
		return nil, fmt.Errorf("failed to get cluster workflow plane client: %w", err)
	}

	return &workflowPlaneInfo{
		client:          wpClient,
		secretStoreName: cwp.Spec.SecretStoreRef.Name,
	}, nil
}

func (s *gitSecretService) buildGitSecret(secretName, ownerNamespace, workflowNamespace, secretType, username, token, sshKey, sshKeyID string) *corev1.Secret {
	var k8sSecretType corev1.SecretType
	var secretData map[string]string

	if secretType == secretTypeBasicAuth {
		k8sSecretType = corev1.SecretTypeBasicAuth
		secretData = map[string]string{
			"password": token,
		}
		if username != "" {
			secretData["username"] = username
		}
	} else {
		k8sSecretType = corev1.SecretTypeSSHAuth
		secretData = map[string]string{
			"ssh-privatekey": sshKey,
		}
		if sshKeyID != "" {
			secretData["ssh-key-id"] = sshKeyID
		}
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: workflowNamespace,
			Labels: map[string]string{
				ownerNamespaceLabel: ownerNamespace,
			},
		},
		Type:       k8sSecretType,
		StringData: secretData,
	}
}

func (s *gitSecretService) buildSecretReference(namespaceName, secretName, secretType, username, sshKeyID, wpKind, wpName string) *openchoreov1alpha1.SecretReference {
	remoteKey := fmt.Sprintf("secret/%s/git/%s", namespaceName, secretName)

	var k8sSecretType corev1.SecretType
	var dataSources []openchoreov1alpha1.SecretDataSource

	if secretType == secretTypeBasicAuth {
		k8sSecretType = corev1.SecretTypeBasicAuth
		dataSources = []openchoreov1alpha1.SecretDataSource{
			{
				SecretKey: "password",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      remoteKey,
					Property: "password",
				},
			},
		}
		if username != "" {
			dataSources = append(dataSources, openchoreov1alpha1.SecretDataSource{
				SecretKey: "username",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      remoteKey,
					Property: "username",
				},
			})
		}
	} else {
		k8sSecretType = corev1.SecretTypeSSHAuth
		dataSources = []openchoreov1alpha1.SecretDataSource{
			{
				SecretKey: "ssh-privatekey",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      remoteKey,
					Property: "ssh-privatekey",
				},
			},
		}
		if sshKeyID != "" {
			dataSources = append(dataSources, openchoreov1alpha1.SecretDataSource{
				SecretKey: "ssh-key-id",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      remoteKey,
					Property: "ssh-key-id",
				},
			})
		}
	}

	return &openchoreov1alpha1.SecretReference{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openchoreov1alpha1.GroupVersion.String(),
			Kind:       "SecretReference",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespaceName,
			Labels: map[string]string{
				gitSecretTypeLabel:     gitSecretTypeValue,
				gitSecretAuthTypeLabel: secretType,
				workflowPlaneKindLabel: wpKind,
				workflowPlaneNameLabel: wpName,
			},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Template: openchoreov1alpha1.SecretTemplate{
				Type: k8sSecretType,
			},
			Data: dataSources,
		},
	}
}

// createPushSecret creates an unstructured PushSecret resource for workflow planes.
func (s *gitSecretService) createPushSecret(name, secretStoreName, ownerNamespace, workflowNamespace, secretType, username, sshKeyID string) *unstructured.Unstructured {
	remoteKey := fmt.Sprintf("secret/%s/git/%s", ownerNamespace, name)

	var dataMatches []map[string]any

	if secretType == secretTypeBasicAuth {
		dataMatches = []map[string]any{
			{
				"match": map[string]any{
					"secretKey": "password",
					"remoteRef": map[string]any{
						"remoteKey": remoteKey,
						"property":  "password",
					},
				},
			},
		}
		if username != "" {
			dataMatches = append(dataMatches, map[string]any{
				"match": map[string]any{
					"secretKey": "username",
					"remoteRef": map[string]any{
						"remoteKey": remoteKey,
						"property":  "username",
					},
				},
			})
		}
	} else {
		dataMatches = []map[string]any{
			{
				"match": map[string]any{
					"secretKey": "ssh-privatekey",
					"remoteRef": map[string]any{
						"remoteKey": remoteKey,
						"property":  "ssh-privatekey",
					},
				},
			},
		}
		if sshKeyID != "" {
			dataMatches = append(dataMatches, map[string]any{
				"match": map[string]any{
					"secretKey": "ssh-key-id",
					"remoteRef": map[string]any{
						"remoteKey": remoteKey,
						"property":  "ssh-key-id",
					},
				},
			})
		}
	}

	pushSecret := &unstructured.Unstructured{}
	pushSecret.SetAPIVersion("external-secrets.io/v1alpha1")
	pushSecret.SetKind("PushSecret")
	pushSecret.SetName(name)
	pushSecret.SetNamespace(workflowNamespace)
	pushSecret.SetLabels(map[string]string{
		ownerNamespaceLabel: ownerNamespace,
	})

	pushSecret.Object["spec"] = map[string]any{
		"updatePolicy": "Replace",
		"secretStoreRefs": []map[string]any{
			{
				"kind": "ClusterSecretStore",
				"name": secretStoreName,
			},
		},
		"selector": map[string]any{
			"secret": map[string]any{
				"name": name,
			},
		},
		"data": dataMatches,
	}
	return pushSecret
}

// ensureNamespaceExists checks if a namespace exists and creates it if not.
func (s *gitSecretService) ensureNamespaceExists(ctx context.Context, k8sClient client.Client, namespaceName string) error {
	namespace := &corev1.Namespace{}
	key := client.ObjectKey{Name: namespaceName}
	if err := k8sClient.Get(ctx, key, namespace); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Info("Creating namespace in workflow plane", "namespace", namespaceName)
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			if err := k8sClient.Create(ctx, namespace); err != nil {
				if apierrors.IsAlreadyExists(err) {
					s.logger.Debug("Namespace already exists (concurrent creation)", "namespace", namespaceName)
					return nil
				}
				s.logger.Error("Failed to create namespace", "error", err, "namespace", namespaceName)
				return fmt.Errorf("failed to create namespace %s: %w", namespaceName, err)
			}
			s.logger.Info("Successfully created namespace in workflow plane", "namespace", namespaceName)
			return nil
		}
		s.logger.Error("Failed to check namespace existence", "error", err, "namespace", namespaceName)
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}
	s.logger.Debug("Namespace already exists in workflow plane", "namespace", namespaceName)
	return nil
}

// validateCredentials checks that the required credential fields are present for the given secret type.
func validateCredentials(req *CreateGitSecretParams) error {
	switch req.SecretType {
	case secretTypeBasicAuth:
		if req.Token == "" {
			return &services.ValidationError{Msg: "token is required for basic-auth type"}
		}
	case secretTypeSSHAuth:
		if req.SSHKey == "" {
			return &services.ValidationError{Msg: "sshKey is required for ssh-auth type"}
		}
	default:
		return &services.ValidationError{Msg: fmt.Sprintf("unsupported secret type: %s", req.SecretType)}
	}
	return nil
}
