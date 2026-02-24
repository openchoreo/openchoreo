// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
)

const (
	maxResponseBytes = 10 * 1024 * 1024 // 10MB

	planeTypeDataPlane          = "dataplane"
	planeTypeBuildPlane         = "buildplane"
	planeTypeObservabilityPlane = "observabilityplane"

	clusterScopeNamespace = "_cluster"
)

// k8sResourceService implements the core query logic without authorization.
type k8sResourceService struct {
	k8sClient     client.Client
	gatewayClient *gateway.Client
	logger        *slog.Logger
}

var _ Service = (*k8sResourceService)(nil)

// NewService creates a new k8s resource query service without authorization.
func NewService(k8sClient client.Client, gatewayClient *gateway.Client, logger *slog.Logger) Service {
	return &k8sResourceService{
		k8sClient:     k8sClient,
		gatewayClient: gatewayClient,
		logger:        logger,
	}
}

func (s *k8sResourceService) QueryK8sResources(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	if s.gatewayClient == nil {
		return nil, ErrGatewayUnavailable
	}

	// Validate k8s resource path
	if err := ValidateK8sPath(req.K8sResourcePath); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidK8sPath, err.Error())
	}

	// Reject streaming parameters
	if req.RawQuery != "" {
		if containsStreamingParam(req.RawQuery) {
			return nil, fmt.Errorf("%w: streaming (watch/follow) is not supported", ErrInvalidK8sPath)
		}
	}

	// Resolve the plane CR to get planeID and determine scope
	planeID, crNamespace, err := s.resolvePlane(ctx, req)
	if err != nil {
		return nil, err
	}

	// Proxy through gateway
	resp, err := s.gatewayClient.ProxyK8sRequest(ctx, req.PlaneType, planeID, crNamespace, req.PlaneName, req.K8sResourcePath, req.RawQuery)
	if err != nil {
		s.logger.Error("Failed to proxy K8s request", "error", err, "planeType", req.PlaneType, "planeID", planeID, "k8sPath", req.K8sResourcePath)
		return nil, fmt.Errorf("%w: %s", ErrGatewayError, err.Error())
	}
	defer resp.Body.Close()

	// Read response body with size limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		s.logger.Error("Failed to read gateway response", "error", err)
		return nil, fmt.Errorf("%w: failed to read response", ErrGatewayError)
	}

	// Unmarshal into generic map
	var data map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &data); err != nil {
			// If we can't unmarshal, return raw body as a string in the data field
			data = map[string]interface{}{"raw": string(body)}
		}
	}

	return &QueryResponse{
		Data:           data,
		K8sStatusCode:  resp.StatusCode,
		PlaneType:      req.PlaneType,
		PlaneName:      req.PlaneName,
		PlaneNamespace: req.PlaneNamespace,
	}, nil
}

// resolvePlane looks up the plane CR and returns (planeID, crNamespace).
func (s *k8sResourceService) resolvePlane(ctx context.Context, req *QueryRequest) (string, string, error) {
	if req.PlaneNamespace != "" {
		// Namespace-scoped plane
		return s.resolveNamespaceScopedPlane(ctx, req.PlaneType, req.PlaneName, req.PlaneNamespace)
	}
	// Cluster-scoped plane
	return s.resolveClusterScopedPlane(ctx, req.PlaneType, req.PlaneName)
}

func (s *k8sResourceService) resolveNamespaceScopedPlane(ctx context.Context, planeType, name, namespace string) (string, string, error) {
	key := client.ObjectKey{Name: name, Namespace: namespace}

	switch planeType {
	case planeTypeDataPlane:
		dp := &openchoreov1alpha1.DataPlane{}
		if err := s.k8sClient.Get(ctx, key, dp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", "", ErrPlaneNotFound
			}
			return "", "", fmt.Errorf("failed to look up DataPlane: %w", err)
		}
		return planeIDOrDefault(dp.Spec.PlaneID, name), namespace, nil

	case planeTypeBuildPlane:
		bp := &openchoreov1alpha1.BuildPlane{}
		if err := s.k8sClient.Get(ctx, key, bp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", "", ErrPlaneNotFound
			}
			return "", "", fmt.Errorf("failed to look up BuildPlane: %w", err)
		}
		return planeIDOrDefault(bp.Spec.PlaneID, name), namespace, nil

	case planeTypeObservabilityPlane:
		op := &openchoreov1alpha1.ObservabilityPlane{}
		if err := s.k8sClient.Get(ctx, key, op); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", "", ErrPlaneNotFound
			}
			return "", "", fmt.Errorf("failed to look up ObservabilityPlane: %w", err)
		}
		return planeIDOrDefault(op.Spec.PlaneID, name), namespace, nil

	default:
		return "", "", fmt.Errorf("%w: unsupported plane type %q", ErrInvalidK8sPath, planeType)
	}
}

func (s *k8sResourceService) resolveClusterScopedPlane(ctx context.Context, planeType, name string) (string, string, error) {
	key := client.ObjectKey{Name: name}

	switch planeType {
	case planeTypeDataPlane:
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		if err := s.k8sClient.Get(ctx, key, cdp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", "", ErrPlaneNotFound
			}
			return "", "", fmt.Errorf("failed to look up ClusterDataPlane: %w", err)
		}
		return planeIDOrDefault(cdp.Spec.PlaneID, name), clusterScopeNamespace, nil

	case planeTypeBuildPlane:
		cbp := &openchoreov1alpha1.ClusterBuildPlane{}
		if err := s.k8sClient.Get(ctx, key, cbp); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", "", ErrPlaneNotFound
			}
			return "", "", fmt.Errorf("failed to look up ClusterBuildPlane: %w", err)
		}
		return planeIDOrDefault(cbp.Spec.PlaneID, name), clusterScopeNamespace, nil

	case planeTypeObservabilityPlane:
		cop := &openchoreov1alpha1.ClusterObservabilityPlane{}
		if err := s.k8sClient.Get(ctx, key, cop); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return "", "", ErrPlaneNotFound
			}
			return "", "", fmt.Errorf("failed to look up ClusterObservabilityPlane: %w", err)
		}
		return planeIDOrDefault(cop.Spec.PlaneID, name), clusterScopeNamespace, nil

	default:
		return "", "", fmt.Errorf("%w: unsupported plane type %q", ErrInvalidK8sPath, planeType)
	}
}

func planeIDOrDefault(planeID, fallback string) string {
	if planeID != "" {
		return planeID
	}
	return fallback
}

// containsStreamingParam checks if the raw query string contains watch=true or follow=true.
func containsStreamingParam(rawQuery string) bool {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false
	}
	return values.Get("watch") == "true" || values.Get("follow") == "true"
}
