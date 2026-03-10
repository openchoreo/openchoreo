// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

//nolint:gosec // False positive: these are annotation keys and namespace prefixes, not credentials
const (
	secretTypeBasicAuth         = "basic-auth"
	secretTypeSSHAuth           = "ssh-auth"
	gitSecretTypeAnnotation     = "openchoreo.dev/secret-type"
	gitSecretTypeValue          = "git-credentials"
	gitSecretAuthTypeAnnotation = "kubernetes.io/secret-type"
	ownerNamespaceLabel         = "openchoreo.dev/owner-namespace"
	gitSecretNamespacePrefix    = "workflows-"
)

// getWorkflowNamespace returns the workflow execution namespace for a given control plane namespace.
func getWorkflowNamespace(namespaceName string) string {
	return gitSecretNamespacePrefix + namespaceName
}

// GitSecretService handles git secret-related business logic
type GitSecretService struct {
	k8sClient            client.Client
	wpClientMgr          *kubernetesClient.KubeMultiClientManager
	workflowPlaneService *WorkflowPlaneService
	logger               *slog.Logger
	authzPDP             authz.PDP
	gatewayURL           string
}

// NewGitSecretService creates a new git secret service
func NewGitSecretService(
	k8sClient client.Client,
	wpClientMgr *kubernetesClient.KubeMultiClientManager,
	workflowPlaneService *WorkflowPlaneService,
	logger *slog.Logger,
	authzPDP authz.PDP,
	gatewayURL string,
) *GitSecretService {
	return &GitSecretService{
		k8sClient:            k8sClient,
		wpClientMgr:          wpClientMgr,
		workflowPlaneService: workflowPlaneService,
		logger:               logger,
		authzPDP:             authzPDP,
		gatewayURL:           gatewayURL,
	}
}

// CreateGitSecret creates a git secret across control and workflow planes
func (s *GitSecretService) CreateGitSecret(ctx context.Context, namespaceName string, req *models.CreateGitSecretRequest) (*models.GitSecretResponse, error) {
	s.logger.Debug("Creating git secret", "namespace", namespaceName, "secret", req.SecretName, "type", req.SecretType)

	req.Sanitize()

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateSecretReference, ResourceTypeSecretReference, req.SecretName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return nil, err
	}

	// Validate secretType
	if req.SecretType != secretTypeBasicAuth && req.SecretType != secretTypeSSHAuth {
		return nil, ErrInvalidSecretType
	}

	// Check if SecretReference already exists in control plane
	existingSecretRef := &openchoreov1alpha1.SecretReference{}
	secretRefKey := client.ObjectKey{Name: req.SecretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, secretRefKey, existingSecretRef); err == nil {
		// SecretReference exists - check if it's a git secret
		if existingSecretRef.Annotations[gitSecretTypeAnnotation] == gitSecretTypeValue {
			s.logger.Warn("Git secret already exists", "namespace", namespaceName, "secret", req.SecretName)
			return nil, ErrGitSecretAlreadyExists
		}
		// Not a git secret, but name collision with another SecretReference
		s.logger.Warn("SecretReference with same name already exists", "namespace", namespaceName, "secret", req.SecretName)
		return nil, ErrGitSecretAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		// Error checking SecretReference (not a NotFound error)
		s.logger.Error("Failed to check existing secret reference", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to check existing secret reference: %w", err)
	}
	// SecretReference doesn't exist, proceed with creation

	workflowPlane, err := s.getWorkflowPlane(ctx, namespaceName)
	if err != nil {
		return nil, err
	}

	secretStoreName := ""
	if workflowPlane.Spec.SecretStoreRef == nil || workflowPlane.Spec.SecretStoreRef.Name == "" {
		s.logger.Warn("Workflow plane has no secret store configured", "namespace", namespaceName, "workflowPlane", workflowPlane.Name)
		return nil, ErrSecretStoreNotConfigured
	}
	secretStoreName = workflowPlane.Spec.SecretStoreRef.Name

	workflowPlaneClient, err := kubernetesClient.GetK8sClientFromWorkflowPlane(s.wpClientMgr, workflowPlane, s.gatewayURL)
	if err != nil {
		s.logger.Error("Failed to get workflow plane client", "error", err, "namespace", namespaceName, "workflowPlane", workflowPlane.Name)
		return nil, fmt.Errorf("failed to get workflow plane client: %w", err)
	}

	// Create namespace: workflows-{namespaceName}
	workflowNamespace := getWorkflowNamespace(namespaceName)
	if err := s.ensureNamespaceExists(ctx, workflowPlaneClient, workflowNamespace); err != nil {
		return nil, err
	}

	// Create or update K8s Secret in workflow plane using Server-Side Apply
	secret := s.buildGitSecret(req.SecretName, namespaceName, workflowNamespace, req.SecretType, req.Username, req.Token, req.SSHKey, req.SSHKEYID)
	if err := workflowPlaneClient.Patch(ctx, secret, client.Apply, client.ForceOwnership, client.FieldOwner("openchoreo-api")); err != nil {
		s.logger.Error("Failed to apply workflow plane secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to apply workflow plane secret: %w", err)
	}
	s.logger.Debug("Successfully applied K8s secret in workflow plane", "namespace", workflowNamespace, "secret", req.SecretName)

	// Create or update PushSecret in workflow plane using Server-Side Apply
	pushSecret := s.createPushSecret(req.SecretName, secretStoreName, namespaceName, workflowNamespace, req.SecretType, req.Username, req.SSHKEYID)
	if err := workflowPlaneClient.Patch(ctx, pushSecret, client.Apply, client.ForceOwnership, client.FieldOwner("openchoreo-api")); err != nil {
		s.logger.Error("Failed to apply push secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to apply push secret: %w", err)
	}
	s.logger.Debug("Successfully applied PushSecret in workflow plane", "namespace", workflowNamespace, "secret", req.SecretName)

	// Create SecretReference in control plane
	secretReference := s.buildSecretReference(namespaceName, req.SecretName, req.SecretType, req.Username, req.SSHKEYID)
	if err := s.k8sClient.Create(ctx, secretReference); err != nil {
		s.logger.Error("Failed to create secret reference", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to create secret reference: %w", err)
	}

	s.logger.Info("Successfully created git secret", "namespace", namespaceName, "secret", req.SecretName, "type", req.SecretType)
	return &models.GitSecretResponse{
		Name:      req.SecretName,
		Namespace: namespaceName,
	}, nil
}

func (s *GitSecretService) getWorkflowPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.WorkflowPlane, error) {
	var workflowPlanes openchoreov1alpha1.WorkflowPlaneList
	if err := s.k8sClient.List(ctx, &workflowPlanes, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list workflow planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list workflow planes: %w", err)
	}

	if len(workflowPlanes.Items) == 0 {
		s.logger.Warn("No workflow planes found", "namespace", namespaceName)
		return nil, ErrWorkflowPlaneNotFound
	}

	return &workflowPlanes.Items[0], nil
}

func (s *GitSecretService) buildGitSecret(secretName, ownerNamespace, workflowNamespace, secretType, username, token, sshKey, sshKeyID string) *corev1.Secret {
	var k8sSecretType corev1.SecretType
	var secretData map[string]string

	if secretType == secretTypeBasicAuth {
		k8sSecretType = corev1.SecretTypeBasicAuth
		secretData = map[string]string{
			"password": token,
		}
		// Add username if provided
		if username != "" {
			secretData["username"] = username
		}
	} else { // secretTypeSSHAuth
		k8sSecretType = corev1.SecretTypeSSHAuth
		secretData = map[string]string{
			"ssh-privatekey": sshKey,
		}
		// Add SSH Key ID if provided (required for AWS CodeCommit)
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

func (s *GitSecretService) buildSecretReference(namespaceName, secretName, secretType, username, sshKeyID string) *openchoreov1alpha1.SecretReference {
	remoteKey := fmt.Sprintf("secret/%s/git/%s", namespaceName, secretName)

	var k8sSecretType corev1.SecretType
	var dataSources []openchoreov1alpha1.SecretDataSource

	if secretType == secretTypeBasicAuth {
		k8sSecretType = corev1.SecretTypeBasicAuth

		// Always add password field
		dataSources = []openchoreov1alpha1.SecretDataSource{
			{
				SecretKey: "password",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      remoteKey,
					Property: "password",
				},
			},
		}

		// Add username field if provided
		if username != "" {
			dataSources = append(dataSources, openchoreov1alpha1.SecretDataSource{
				SecretKey: "username",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      remoteKey,
					Property: "username",
				},
			})
		}
	} else { // secretTypeSSHAuth
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

		// Add SSH Key ID if provided (required for AWS CodeCommit)
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
			Annotations: map[string]string{
				gitSecretAuthTypeAnnotation: secretType,
				gitSecretTypeAnnotation:     gitSecretTypeValue,
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
func (s *GitSecretService) createPushSecret(name, secretStoreName, ownerNamespace, workflowNamespace, secretType, username, sshKeyID string) *unstructured.Unstructured {
	remoteKey := fmt.Sprintf("secret/%s/git/%s", ownerNamespace, name)

	var dataMatches []map[string]interface{}

	if secretType == secretTypeBasicAuth {
		// Always add password field
		dataMatches = []map[string]interface{}{
			{
				"match": map[string]interface{}{
					"secretKey": "password",
					"remoteRef": map[string]interface{}{
						"remoteKey": remoteKey,
						"property":  "password",
					},
				},
			},
		}

		// Add username field if provided
		if username != "" {
			dataMatches = append(dataMatches, map[string]interface{}{
				"match": map[string]interface{}{
					"secretKey": "username",
					"remoteRef": map[string]interface{}{
						"remoteKey": remoteKey,
						"property":  "username",
					},
				},
			})
		}
	} else { // secretTypeSSHAuth
		dataMatches = []map[string]interface{}{
			{
				"match": map[string]interface{}{
					"secretKey": "ssh-privatekey",
					"remoteRef": map[string]interface{}{
						"remoteKey": remoteKey,
						"property":  "ssh-privatekey",
					},
				},
			},
		}

		// Add SSH Key ID if provided (required for AWS CodeCommit)
		if sshKeyID != "" {
			dataMatches = append(dataMatches, map[string]interface{}{
				"match": map[string]interface{}{
					"secretKey": "ssh-key-id",
					"remoteRef": map[string]interface{}{
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

	pushSecret.Object["spec"] = map[string]interface{}{
		"updatePolicy": "Replace",
		// "deletionPolicy": "Delete",
		"secretStoreRefs": []map[string]interface{}{
			{
				"kind": "ClusterSecretStore",
				"name": secretStoreName,
			},
		},
		"selector": map[string]interface{}{
			"secret": map[string]interface{}{
				"name": name,
			},
		},
		"data": dataMatches,
	}
	return pushSecret
}

// ensureNamespaceExists checks if a namespace exists and creates it if not
func (s *GitSecretService) ensureNamespaceExists(ctx context.Context, k8sClient client.Client, namespaceName string) error {
	namespace := &corev1.Namespace{}
	key := client.ObjectKey{Name: namespaceName}
	if err := k8sClient.Get(ctx, key, namespace); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Namespace doesn't exist, create it
			s.logger.Info("Creating namespace in workflow plane", "namespace", namespaceName)
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			if err := k8sClient.Create(ctx, namespace); err != nil {
				s.logger.Error("Failed to create namespace", "error", err, "namespace", namespaceName)
				return fmt.Errorf("failed to create namespace %s: %w", namespaceName, err)
			}
			s.logger.Info("Successfully created namespace in workflow plane", "namespace", namespaceName)
			return nil
		}
		s.logger.Error("Failed to check namespace existence", "error", err, "namespace", namespaceName)
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}
	// Namespace already exists
	s.logger.Debug("Namespace already exists in workflow plane", "namespace", namespaceName)
	return nil
}

// ListGitSecrets lists all git secrets for a namespace
func (s *GitSecretService) ListGitSecrets(ctx context.Context, namespaceName string) ([]models.GitSecretResponse, error) {
	s.logger.Debug("Listing git secrets", "namespace", namespaceName)

	// List SecretReference CRDs with git-credentials annotation in the namespace
	var secretRefs openchoreov1alpha1.SecretReferenceList
	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if err := s.k8sClient.List(ctx, &secretRefs, listOpts...); err != nil {
		s.logger.Error("Failed to list secret references", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list git secrets: %w", err)
	}

	secrets := make([]models.GitSecretResponse, 0, len(secretRefs.Items))
	for _, ref := range secretRefs.Items {
		// Filter by annotation (since we can't use MatchingLabels with annotations)
		if ref.Annotations[gitSecretTypeAnnotation] == gitSecretTypeValue {
			// Authorization check for each secret
			if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionViewSecretReference, ResourceTypeSecretReference, ref.Name,
				authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
				if errors.Is(err, ErrForbidden) {
					// Skip unauthorized secrets silently
					s.logger.Debug("Skipping unauthorized git secret", "namespace", namespaceName, "secret", ref.Name)
					continue
				}
				// Return system errors
				return nil, err
			}
			secrets = append(secrets, models.GitSecretResponse{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			})
		}
	}

	return secrets, nil
}

// DeleteGitSecret deletes a git secret from control and workflow planes
func (s *GitSecretService) DeleteGitSecret(ctx context.Context, namespaceName, secretName string) error {
	s.logger.Debug("Deleting git secret", "namespace", namespaceName, "secret", secretName)

	// Authorization check
	if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionDeleteSecretReference, ResourceTypeSecretReference, secretName,
		authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
		return err
	}

	// First, verify the secret reference exists
	secretRef := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{Name: secretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, secretRef); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrGitSecretNotFound
		}
		s.logger.Error("Failed to get secret reference", "error", err, "namespace", namespaceName, "secret", secretName)
		return fmt.Errorf("failed to get secret reference: %w", err)
	}

	// Verify it's a git secret by checking the annotation
	if secretRef.Annotations[gitSecretTypeAnnotation] != gitSecretTypeValue {
		return ErrGitSecretNotFound
	}

	// Get workflow plane to delete resources from workflow plane
	workflowPlane, err := s.getWorkflowPlane(ctx, namespaceName)
	if err != nil {
		return err
	}

	workflowPlaneClient, err := kubernetesClient.GetK8sClientFromWorkflowPlane(s.wpClientMgr, workflowPlane, s.gatewayURL)
	if err != nil {
		s.logger.Error("Failed to get workflow plane client", "error", err, "namespace", namespaceName, "workflowPlane", workflowPlane.Name)
		return fmt.Errorf("failed to get workflow plane client: %w", err)
	}

	workflowNamespace := getWorkflowNamespace(namespaceName)

	// Delete PushSecret from workflow plane
	pushSecret := &unstructured.Unstructured{}
	pushSecret.SetAPIVersion("external-secrets.io/v1alpha1")
	pushSecret.SetKind("PushSecret")
	pushSecret.SetName(secretName)
	pushSecret.SetNamespace(workflowNamespace)
	if err := workflowPlaneClient.Delete(ctx, pushSecret); err != nil {
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
	if err := workflowPlaneClient.Delete(ctx, secret); err != nil {
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
