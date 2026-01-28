// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
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

const (
	gitSecretSystemNamespace = "openchoreo-system"
	gitSecretTypeLabel       = "openchoreo.dev/secret-type"
	gitSecretTypeValue       = "git-credentials"
	ownerNamespaceLabel      = "openchoreo.dev/owner-namespace"
)

// GitSecretService handles git secret-related business logic
type GitSecretService struct {
	k8sClient         client.Client
	bpClientMgr       *kubernetesClient.KubeMultiClientManager
	buildPlaneService *BuildPlaneService
	logger            *slog.Logger
	authzPDP          authz.PDP
	gatewayURL        string
}

// NewGitSecretService creates a new git secret service
func NewGitSecretService(
	k8sClient client.Client,
	bpClientMgr *kubernetesClient.KubeMultiClientManager,
	buildPlaneService *BuildPlaneService,
	logger *slog.Logger,
	authzPDP authz.PDP,
	gatewayURL string,
) *GitSecretService {
	return &GitSecretService{
		k8sClient:         k8sClient,
		bpClientMgr:       bpClientMgr,
		buildPlaneService: buildPlaneService,
		logger:            logger,
		authzPDP:          authzPDP,
		gatewayURL:        gatewayURL,
	}
}

// CreateGitSecret creates a git secret across control and build planes
func (s *GitSecretService) CreateGitSecret(ctx context.Context, namespaceName string, req *models.CreateGitSecretRequest) (*models.GitSecretResponse, error) {
	s.logger.Debug("Creating git secret", "namespace", namespaceName, "secret", req.SecretName)

	req.Sanitize()

	// if err := checkAuthorization(ctx, s.logger, s.authzPDP, SystemActionCreateGitSecret, ResourceTypeGitSecret, req.SecretName,
	//	authz.ResourceHierarchy{Namespace: namespaceName}); err != nil {
	//	return nil, err
	//}

	buildPlane, err := s.getBuildPlane(ctx, namespaceName)
	if err != nil {
		return nil, err
	}

	secretStoreName := ""
	if buildPlane.Spec.SecretStoreRef == nil || buildPlane.Spec.SecretStoreRef.Name == "" {
		s.logger.Warn("Build plane has no secret store configured", "namespace", namespaceName, "buildPlane", buildPlane.Name)
		return nil, ErrSecretStoreNotConfigured
	}
	secretStoreName = buildPlane.Spec.SecretStoreRef.Name

	buildPlaneClient, err := kubernetesClient.GetK8sClientFromBuildPlane(s.bpClientMgr, buildPlane, s.gatewayURL)
	if err != nil {
		s.logger.Error("Failed to get build plane client", "error", err, "namespace", namespaceName, "buildPlane", buildPlane.Name)
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	// Ensure openchoreo-system namespace exists in build plane
	if err := s.ensureNamespaceExists(ctx, buildPlaneClient, gitSecretSystemNamespace); err != nil {
		return nil, err
	}

	secret := s.buildGitSecret(req.SecretName, namespaceName, req.Token)
	if err := buildPlaneClient.Create(ctx, secret); err != nil {
		s.logger.Error("Failed to create build plane secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to create build plane secret: %w", err)
	}

	pushSecret := s.createPushSecret(req.SecretName, secretStoreName, namespaceName)
	if err := buildPlaneClient.Create(ctx, pushSecret); err != nil {
		s.logger.Error("Failed to create push secret", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to create push secret: %w", err)
	}

	secretReference := s.buildSecretReference(namespaceName, req.SecretName)
	if err := s.k8sClient.Create(ctx, secretReference); err != nil {
		s.logger.Error("Failed to create secret reference", "error", err, "namespace", namespaceName, "secret", req.SecretName)
		return nil, fmt.Errorf("failed to create secret reference: %w", err)
	}

	return &models.GitSecretResponse{
		Name:      req.SecretName,
		Namespace: namespaceName,
	}, nil
}

func (s *GitSecretService) getBuildPlane(ctx context.Context, namespaceName string) (*openchoreov1alpha1.BuildPlane, error) {
	var buildPlanes openchoreov1alpha1.BuildPlaneList
	if err := s.k8sClient.List(ctx, &buildPlanes, client.InNamespace(namespaceName)); err != nil {
		s.logger.Error("Failed to list build planes", "error", err, "namespace", namespaceName)
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	if len(buildPlanes.Items) == 0 {
		s.logger.Warn("No build planes found", "namespace", namespaceName)
		return nil, ErrBuildPlaneNotFound
	}

	return &buildPlanes.Items[0], nil
}

func (s *GitSecretService) buildGitSecret(secretName, ownerNamespace, token string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: gitSecretSystemNamespace,
			Labels: map[string]string{
				ownerNamespaceLabel: ownerNamespace,
				gitSecretTypeLabel:  gitSecretTypeValue,
			},
		},
		Type: corev1.SecretTypeBasicAuth,
		StringData: map[string]string{
			"password": token,
		},
	}
}

func (s *GitSecretService) buildSecretReference(namespaceName, secretName string) *openchoreov1alpha1.SecretReference {
	remoteKey := fmt.Sprintf("%s/git/%s", namespaceName, secretName)
	return &openchoreov1alpha1.SecretReference{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openchoreov1alpha1.GroupVersion.String(),
			Kind:       "SecretReference",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespaceName,
			Labels: map[string]string{
				gitSecretTypeLabel: gitSecretTypeValue,
			},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Template: openchoreov1alpha1.SecretTemplate{
				Type: corev1.SecretTypeBasicAuth,
			},
			Data: []openchoreov1alpha1.SecretDataSource{
				{
					SecretKey: "password",
					RemoteRef: openchoreov1alpha1.RemoteReference{
						Key:      remoteKey,
						Property: "password",
					},
				},
			},
		},
	}
}

// createPushSecret creates an unstructured PushSecret resource for build planes.
func (s *GitSecretService) createPushSecret(name, secretStoreName, ownerNamespace string) *unstructured.Unstructured {
	remoteKey := fmt.Sprintf("secret/data/%s/git/%s", ownerNamespace, name)
	pushSecret := &unstructured.Unstructured{}
	pushSecret.SetAPIVersion("external-secrets.io/v1alpha1")
	pushSecret.SetKind("PushSecret")
	pushSecret.SetName(name)
	pushSecret.SetNamespace(gitSecretSystemNamespace)
	pushSecret.SetLabels(map[string]string{
		ownerNamespaceLabel: ownerNamespace,
		gitSecretTypeLabel:  gitSecretTypeValue,
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
		"data": []map[string]interface{}{
			{
				"match": map[string]interface{}{
					"secretKey": "password",
					"remoteRef": map[string]interface{}{
						"remoteKey": remoteKey,
						"property":  "password",
					},
				},
			},
		},
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
			s.logger.Info("Creating namespace in build plane", "namespace", namespaceName)
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			if err := k8sClient.Create(ctx, namespace); err != nil {
				s.logger.Error("Failed to create namespace", "error", err, "namespace", namespaceName)
				return fmt.Errorf("failed to create namespace %s: %w", namespaceName, err)
			}
			s.logger.Info("Successfully created namespace in build plane", "namespace", namespaceName)
			return nil
		}
		s.logger.Error("Failed to check namespace existence", "error", err, "namespace", namespaceName)
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}
	// Namespace already exists
	s.logger.Debug("Namespace already exists in build plane", "namespace", namespaceName)
	return nil
}
