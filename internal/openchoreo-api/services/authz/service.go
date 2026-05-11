// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// authzService handles authz CRUD operations without authorization checks.
type authzService struct {
	pap    authzcore.PAP
	pdp    authzcore.PDP
	logger *slog.Logger
}

var _ Service = (*authzService)(nil)

var clusterAuthzRoleTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterAuthzRole",
}

var authzRoleTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "AuthzRole",
}

var clusterAuthzRoleBindingTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "ClusterAuthzRoleBinding",
}

var authzRoleBindingTypeMeta = metav1.TypeMeta{
	APIVersion: openchoreov1alpha1.GroupVersion.String(),
	Kind:       "AuthzRoleBinding",
}

// NewService creates a new authz service without authorization checks.
func NewService(pap authzcore.PAP, pdp authzcore.PDP, logger *slog.Logger) Service {
	return &authzService{
		pap:    pap,
		pdp:    pdp,
		logger: logger,
	}
}

// --- Cluster Roles ---

func (s *authzService) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("cluster role cannot be nil")
	}
	s.logger.Debug("Creating cluster role", "name", role.Name)
	created, err := s.pap.CreateClusterRole(ctx, role)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	created.TypeMeta = clusterAuthzRoleTypeMeta
	return created, nil
}

func (s *authzService) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	s.logger.Debug("Getting cluster role", "name", name)
	role, err := s.pap.GetClusterRole(ctx, name)
	if err != nil {
		return nil, err
	}
	role.TypeMeta = clusterAuthzRoleTypeMeta
	return role, nil
}

func (s *authzService) ListClusterRoles(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterAuthzRole], error) {
	s.logger.Debug("Listing cluster roles", "limit", opts.Limit, "cursor", opts.Cursor)
	paged, err := s.pap.ListClusterRoles(ctx, opts.Limit, opts.Cursor)
	if err != nil {
		return nil, err
	}
	for i := range paged.Items {
		paged.Items[i].TypeMeta = clusterAuthzRoleTypeMeta
	}
	return &services.ListResult[openchoreov1alpha1.ClusterAuthzRole]{
		Items:      paged.Items,
		NextCursor: paged.NextCursor,
	}, nil
}

func (s *authzService) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.ClusterAuthzRole) (*openchoreov1alpha1.ClusterAuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("cluster role cannot be nil")
	}
	s.logger.Debug("Updating cluster role", "name", role.Name)
	updated, err := s.pap.UpdateClusterRole(ctx, role)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	updated.TypeMeta = clusterAuthzRoleTypeMeta
	return updated, nil
}

func (s *authzService) DeleteClusterRole(ctx context.Context, name string) error {
	s.logger.Debug("Deleting cluster role", "name", name)
	return s.pap.DeleteClusterRole(ctx, name)
}

// --- Namespace Roles ---

func (s *authzService) CreateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("namespace role cannot be nil")
	}
	role.Namespace = namespace
	s.logger.Debug("Creating namespace role", "namespace", namespace, "name", role.Name)
	created, err := s.pap.CreateNamespacedRole(ctx, role)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	created.TypeMeta = authzRoleTypeMeta
	return created, nil
}

func (s *authzService) GetNamespaceRole(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRole, error) {
	s.logger.Debug("Getting namespace role", "namespace", namespace, "name", name)
	role, err := s.pap.GetNamespacedRole(ctx, name, namespace)
	if err != nil {
		return nil, err
	}
	role.TypeMeta = authzRoleTypeMeta
	return role, nil
}

func (s *authzService) ListNamespaceRoles(ctx context.Context, namespace string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.AuthzRole], error) {
	s.logger.Debug("Listing namespace roles", "namespace", namespace, "limit", opts.Limit, "cursor", opts.Cursor)
	paged, err := s.pap.ListNamespacedRoles(ctx, namespace, opts.Limit, opts.Cursor)
	if err != nil {
		return nil, err
	}
	for i := range paged.Items {
		paged.Items[i].TypeMeta = authzRoleTypeMeta
	}
	return &services.ListResult[openchoreov1alpha1.AuthzRole]{
		Items:      paged.Items,
		NextCursor: paged.NextCursor,
	}, nil
}

func (s *authzService) UpdateNamespaceRole(ctx context.Context, namespace string, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	if role == nil {
		return nil, fmt.Errorf("namespace role cannot be nil")
	}
	role.Namespace = namespace
	s.logger.Debug("Updating namespace role", "namespace", namespace, "name", role.Name)
	updated, err := s.pap.UpdateNamespacedRole(ctx, role)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	updated.TypeMeta = authzRoleTypeMeta
	return updated, nil
}

func (s *authzService) DeleteNamespaceRole(ctx context.Context, namespace, name string) error {
	s.logger.Debug("Deleting namespace role", "namespace", namespace, "name", name)
	return s.pap.DeleteNamespacedRole(ctx, name, namespace)
}

// --- Cluster Role Bindings ---

func (s *authzService) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("cluster role binding cannot be nil")
	}
	s.logger.Debug("Creating cluster role binding", "name", binding.Name)
	created, err := s.pap.CreateClusterRoleBinding(ctx, binding)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	created.TypeMeta = clusterAuthzRoleBindingTypeMeta
	return created, nil
}

func (s *authzService) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	s.logger.Debug("Getting cluster role binding", "name", name)
	binding, err := s.pap.GetClusterRoleBinding(ctx, name)
	if err != nil {
		return nil, err
	}
	binding.TypeMeta = clusterAuthzRoleBindingTypeMeta
	return binding, nil
}

func (s *authzService) ListClusterRoleBindings(ctx context.Context, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding], error) {
	s.logger.Debug("Listing cluster role bindings", "limit", opts.Limit, "cursor", opts.Cursor)
	paged, err := s.pap.ListClusterRoleBindings(ctx, opts.Limit, opts.Cursor)
	if err != nil {
		return nil, err
	}
	for i := range paged.Items {
		paged.Items[i].TypeMeta = clusterAuthzRoleBindingTypeMeta
	}
	return &services.ListResult[openchoreov1alpha1.ClusterAuthzRoleBinding]{
		Items:      paged.Items,
		NextCursor: paged.NextCursor,
	}, nil
}

func (s *authzService) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.ClusterAuthzRoleBinding) (*openchoreov1alpha1.ClusterAuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("cluster role binding cannot be nil")
	}
	s.logger.Debug("Updating cluster role binding", "name", binding.Name)
	updated, err := s.pap.UpdateClusterRoleBinding(ctx, binding)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	updated.TypeMeta = clusterAuthzRoleBindingTypeMeta
	return updated, nil
}

func (s *authzService) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	s.logger.Debug("Deleting cluster role binding", "name", name)
	return s.pap.DeleteClusterRoleBinding(ctx, name)
}

// --- Namespace Role Bindings ---

func (s *authzService) CreateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("namespace role binding cannot be nil")
	}
	binding.Namespace = namespace
	s.logger.Debug("Creating namespace role binding", "namespace", namespace, "name", binding.Name)
	created, err := s.pap.CreateNamespacedRoleBinding(ctx, binding)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	created.TypeMeta = authzRoleBindingTypeMeta
	return created, nil
}

func (s *authzService) GetNamespaceRoleBinding(ctx context.Context, namespace, name string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	s.logger.Debug("Getting namespace role binding", "namespace", namespace, "name", name)
	binding, err := s.pap.GetNamespacedRoleBinding(ctx, name, namespace)
	if err != nil {
		return nil, err
	}
	binding.TypeMeta = authzRoleBindingTypeMeta
	return binding, nil
}

func (s *authzService) ListNamespaceRoleBindings(ctx context.Context, namespace string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.AuthzRoleBinding], error) {
	s.logger.Debug("Listing namespace role bindings", "namespace", namespace, "limit", opts.Limit, "cursor", opts.Cursor)
	paged, err := s.pap.ListNamespacedRoleBindings(ctx, namespace, opts.Limit, opts.Cursor)
	if err != nil {
		return nil, err
	}
	for i := range paged.Items {
		paged.Items[i].TypeMeta = authzRoleBindingTypeMeta
	}
	return &services.ListResult[openchoreov1alpha1.AuthzRoleBinding]{
		Items:      paged.Items,
		NextCursor: paged.NextCursor,
	}, nil
}

func (s *authzService) UpdateNamespaceRoleBinding(ctx context.Context, namespace string, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	if binding == nil {
		return nil, fmt.Errorf("namespace role binding cannot be nil")
	}
	binding.Namespace = namespace
	s.logger.Debug("Updating namespace role binding", "namespace", namespace, "name", binding.Name)
	updated, err := s.pap.UpdateNamespacedRoleBinding(ctx, binding)
	if err != nil {
		if vErr := services.ExtractValidationError(err); vErr != nil {
			return nil, vErr
		}
		return nil, err
	}
	updated.TypeMeta = authzRoleBindingTypeMeta
	return updated, nil
}

func (s *authzService) DeleteNamespaceRoleBinding(ctx context.Context, namespace, name string) error {
	s.logger.Debug("Deleting namespace role binding", "namespace", namespace, "name", name)
	return s.pap.DeleteNamespacedRoleBinding(ctx, name, namespace)
}

// --- Evaluation & Profile ---

// Evaluate evaluates one or more authorization requests using the PDP.
func (s *authzService) Evaluate(ctx context.Context, requests []authzcore.EvaluateRequest) ([]authzcore.Decision, error) {
	s.logger.Debug("Evaluating authorization requests", "count", len(requests))
	batchResp, err := s.pdp.BatchEvaluate(ctx, &authzcore.BatchEvaluateRequest{Requests: requests})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate authorization requests: %w", err)
	}
	return batchResp.Decisions, nil
}

// ListActions lists all public actions in the system.
func (s *authzService) ListActions(ctx context.Context) ([]authzcore.Action, error) {
	s.logger.Debug("Listing actions")
	return s.pap.ListActions(ctx)
}

// GetSubjectProfile retrieves the authorization profile for a given subject.
func (s *authzService) GetSubjectProfile(ctx context.Context, request *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	s.logger.Debug("Getting subject profile")
	return s.pdp.GetSubjectProfile(ctx, request)
}
