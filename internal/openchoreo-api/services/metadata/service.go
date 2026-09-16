// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

type metadataService struct {
	k8sClient    client.Client
	auditEnabled bool
	auditRef     config.AuditObservabilityPlaneRef
	logger       *slog.Logger
}

var _ Service = (*metadataService)(nil)

// NewService creates a metadata service. It has no authorization wrapper
// because the metadata endpoint requires no permission.
func NewService(k8sClient client.Client, auditCfg config.AuditConfig, logger *slog.Logger) Service {
	return &metadataService{
		k8sClient:    k8sClient,
		auditEnabled: auditCfg.Enabled,
		auditRef:     auditCfg.ObservabilityPlaneRef,
		logger:       logger,
	}
}

func (s *metadataService) GetMetadata(ctx context.Context) (*Metadata, error) {
	auditLogs, err := s.getAuditLogs(ctx)
	if err != nil {
		return nil, err
	}
	return &Metadata{AuditLogs: auditLogs}, nil
}

func (s *metadataService) getAuditLogs(ctx context.Context) (AuditLogs, error) {
	if !s.auditEnabled {
		return AuditLogs{}, nil
	}

	auditLogs := AuditLogs{Enabled: true}
	ref := s.auditRef
	plane, err := controller.GetObservabilityPlaneFromRef(ctx, s.k8sClient, ref.Namespace, &openchoreov1alpha1.ObservabilityPlaneRef{
		Kind: openchoreov1alpha1.ObservabilityPlaneRefKind(ref.Kind),
		Name: ref.Name,
	})
	if err != nil {
		// A missing plane leaves out the observer URL instead of failing the whole response.
		if apierrors.IsNotFound(err) {
			s.logger.Warn("Audit observability plane not found; observer URL is not advertised",
				"kind", ref.Kind, "namespace", ref.Namespace, "name", ref.Name)
			return auditLogs, nil
		}
		return AuditLogs{}, fmt.Errorf("failed to resolve the audit observability plane: %w", err)
	}

	auditLogs.ObserverURL = plane.GetObserverURL()
	return auditLogs, nil
}
