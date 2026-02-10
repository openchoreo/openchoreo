// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// planeInfo holds extracted information from a plane CR for CA processing
type planeInfo struct {
	name      string
	namespace string
	planeID   string
	agent     *openchoreov1alpha1.ClusterAgentConfig
}

// dataPlaneInfo extracts plane information from a DataPlane CR
// Returns the extracted info and true if the CR should be processed
func dataPlaneInfo(obj client.Object) (planeInfo, bool) {
	dp, ok := obj.(*openchoreov1alpha1.DataPlane)
	if !ok {
		return planeInfo{}, false
	}

	planeID := dp.Spec.PlaneID
	if planeID == "" {
		planeID = dp.Name
	}

	return planeInfo{
		name:      dp.Name,
		namespace: dp.Namespace,
		planeID:   planeID,
		agent:     &dp.Spec.ClusterAgent,
	}, true
}

// buildPlaneInfo extracts plane information from a BuildPlane CR
// Returns the extracted info and true if the CR should be processed
func buildPlaneInfo(obj client.Object) (planeInfo, bool) {
	bp, ok := obj.(*openchoreov1alpha1.BuildPlane)
	if !ok {
		return planeInfo{}, false
	}

	planeID := bp.Spec.PlaneID
	if planeID == "" {
		planeID = bp.Name
	}

	return planeInfo{
		name:      bp.Name,
		namespace: bp.Namespace,
		planeID:   planeID,
		agent:     &bp.Spec.ClusterAgent,
	}, true
}

// obsPlaneInfo extracts plane information from an ObservabilityPlane CR
// Returns the extracted info and true if the CR should be processed
func obsPlaneInfo(obj client.Object) (planeInfo, bool) {
	op, ok := obj.(*openchoreov1alpha1.ObservabilityPlane)
	if !ok {
		return planeInfo{}, false
	}

	planeID := op.Spec.PlaneID
	if planeID == "" {
		planeID = op.Name
	}

	return planeInfo{
		name:      op.Name,
		namespace: op.Namespace,
		planeID:   planeID,
		agent:     &op.Spec.ClusterAgent,
	}, true
}

// clusterDataPlaneInfo extracts plane information from a ClusterDataPlane CR
// Returns the extracted info and true if the CR should be processed
func clusterDataPlaneInfo(obj client.Object) (planeInfo, bool) {
	cdp, ok := obj.(*openchoreov1alpha1.ClusterDataPlane)
	if !ok {
		return planeInfo{}, false
	}

	return planeInfo{
		name:      cdp.Name,
		namespace: "", // Cluster-scoped - no namespace
		planeID:   cdp.Spec.PlaneID,
		agent:     &cdp.Spec.ClusterAgent,
	}, true
}

// clusterBuildPlaneInfo extracts plane information from a ClusterBuildPlane CR
// Returns the extracted info and true if the CR should be processed
func clusterBuildPlaneInfo(obj client.Object) (planeInfo, bool) {
	cbp, ok := obj.(*openchoreov1alpha1.ClusterBuildPlane)
	if !ok {
		return planeInfo{}, false
	}

	return planeInfo{
		name:      cbp.Name,
		namespace: "", // Cluster-scoped - no namespace
		planeID:   cbp.Spec.PlaneID,
		agent:     &cbp.Spec.ClusterAgent,
	}, true
}

// clusterObsPlaneInfo extracts plane information from a ClusterObservabilityPlane CR
// Returns the extracted info and true if the CR should be processed
func clusterObsPlaneInfo(obj client.Object) (planeInfo, bool) {
	cop, ok := obj.(*openchoreov1alpha1.ClusterObservabilityPlane)
	if !ok {
		return planeInfo{}, false
	}

	return planeInfo{
		name:      cop.Name,
		namespace: "", // Cluster-scoped - no namespace
		planeID:   cop.Spec.PlaneID,
		agent:     &cop.Spec.ClusterAgent,
	}, true
}

// extractPlaneClientCAs is a generic helper that extracts client CAs from a list of plane CRs
// It eliminates code duplication across DataPlane, BuildPlane, and ObservabilityPlane processing
func (s *Server) extractPlaneClientCAs(
	ctx context.Context,
	planeType string,
	planeID string,
	list client.ObjectList,
	extractInfo func(client.Object) (planeInfo, bool),
) (map[string][]byte, error) {
	result := make(map[string][]byte)

	if err := s.k8sClient.List(ctx, list); err != nil {
		return nil, fmt.Errorf("failed to list %s: %w", planeType, err)
	}

	// Extract items from the list
	// All plane list types have an Items field that implements client.Object
	items, err := extractListItems(list)
	if err != nil {
		return nil, fmt.Errorf("failed to extract items from list: %w", err)
	}

	for _, item := range items {
		info, ok := extractInfo(item)
		if !ok {
			continue // Skip items that can't be extracted (shouldn't happen)
		}

		if info.planeID != planeID {
			continue
		}

		crKey := fmt.Sprintf("%s/%s", info.namespace, info.name)
		caData, err := s.extractCAFromPlane(info.agent, info.namespace)
		if err != nil {
			s.logger.Warn("failed to extract CA from plane CR",
				"planeType", planeType,
				"namespace", info.namespace,
				"name", info.name,
				"error", err,
			)
			continue
		}

		result[crKey] = caData
	}

	return result, nil
}

// extractListItems extracts the Items slice from a client.ObjectList
// This handles the type assertion for DataPlaneList, BuildPlaneList, ObservabilityPlaneList
func extractListItems(list client.ObjectList) ([]client.Object, error) {
	switch v := list.(type) {
	case *openchoreov1alpha1.DataPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	case *openchoreov1alpha1.BuildPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	case *openchoreov1alpha1.ObservabilityPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	case *openchoreov1alpha1.ClusterDataPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	case *openchoreov1alpha1.ClusterBuildPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	case *openchoreov1alpha1.ClusterObservabilityPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported list type: %T", list)
	}
}

// getAllPlaneClientCAs retrieves client CA configurations from ALL CRs with matching planeType and planeID
// Returns map of "namespace/name" -> CA data ([]byte)
// For namespace-scoped CRs, key format is "namespace/name"
// For cluster-scoped CRs, key format is "/name" (empty namespace)
func (s *Server) getAllPlaneClientCAs(planeType, planeID string) (map[string][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := make(map[string][]byte)

	// Query both namespace-scoped and cluster-scoped CRs for each plane type
	switch planeType {
	case planeTypeDataPlane:
		// Namespace-scoped DataPlane
		nsResult, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.DataPlaneList{}, dataPlaneInfo)
		if err != nil {
			return nil, err
		}
		for k, v := range nsResult {
			result[k] = v
		}
		// Cluster-scoped ClusterDataPlane
		clusterResult, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ClusterDataPlaneList{}, clusterDataPlaneInfo)
		if err != nil {
			return nil, err
		}
		for k, v := range clusterResult {
			result[k] = v
		}

	case planeTypeBuildPlane:
		// Namespace-scoped BuildPlane
		nsResult, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.BuildPlaneList{}, buildPlaneInfo)
		if err != nil {
			return nil, err
		}
		for k, v := range nsResult {
			result[k] = v
		}
		// Cluster-scoped ClusterBuildPlane
		clusterResult, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ClusterBuildPlaneList{}, clusterBuildPlaneInfo)
		if err != nil {
			return nil, err
		}
		for k, v := range clusterResult {
			result[k] = v
		}

	case planeTypeObservabilityPlane:
		// Namespace-scoped ObservabilityPlane
		nsResult, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ObservabilityPlaneList{}, obsPlaneInfo)
		if err != nil {
			return nil, err
		}
		for k, v := range nsResult {
			result[k] = v
		}
		// Cluster-scoped ClusterObservabilityPlane
		clusterResult, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ClusterObservabilityPlaneList{}, clusterObsPlaneInfo)
		if err != nil {
			return nil, err
		}
		for k, v := range clusterResult {
			result[k] = v
		}

	default:
		return nil, fmt.Errorf("unsupported plane type: %s", planeType)
	}

	s.logger.Info("retrieved client CAs from CRs",
		"planeType", planeType,
		"planeID", planeID,
		"totalCRs", len(result),
	)

	if len(result) > 0 {
		for crKey := range result {
			s.logger.Info("discovered CR for plane",
				"planeType", planeType,
				"planeID", planeID,
				"cr", crKey,
			)
		}
	} else {
		s.logger.Warn("no CRs found for connecting agent",
			"planeType", planeType,
			"planeID", planeID,
			"note", "agent will connect but without proper CA verification",
		)
	}

	return result, nil
}

// extractCAFromPlane extracts CA data from a plane's ClusterAgent configuration
func (s *Server) extractCAFromPlane(clusterAgent *openchoreov1alpha1.ClusterAgentConfig, namespace string) ([]byte, error) {
	if clusterAgent == nil {
		return nil, nil
	}
	return s.extractCADataWithNamespace(&clusterAgent.ClientCA, namespace)
}

// extractCAData extracts CA certificate data from ValueFrom configuration
// planeNamespace is used as default namespace for SecretRef if not specified
func (s *Server) extractCADataWithNamespace(valueFrom *openchoreov1alpha1.ValueFrom, planeNamespace string) ([]byte, error) {
	if valueFrom.Value != "" {
		return []byte(valueFrom.Value), nil
	}

	if valueFrom.SecretRef != nil {
		return s.extractCAFromSecret(valueFrom.SecretRef, planeNamespace)
	}

	return nil, fmt.Errorf("no valid CA data found in ValueFrom")
}

func parseCACertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate

	for len(pemData) > 0 {
		block, rest := pem.Decode(pemData)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			certs = append(certs, cert)
		}

		pemData = rest
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}

	return certs, nil
}

// extractCAFromSecret extracts CA certificate data from a Kubernetes secret
// planeNamespace is used as default if secretRef.Namespace is not specified
func (s *Server) extractCAFromSecret(secretRef *openchoreov1alpha1.SecretKeyReference, planeNamespace string) ([]byte, error) {
	if secretRef.Name == "" {
		return nil, fmt.Errorf("secret name is required")
	}

	if secretRef.Key == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = planeNamespace
	}

	s.logger.Debug("loading CA certificate from secret",
		"secretName", secretRef.Name,
		"namespace", namespace,
		"key", secretRef.Key,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: namespace,
	}

	if err := s.k8sClient.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	caData, ok := secret.Data[secretRef.Key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret %s/%s", secretRef.Key, namespace, secretRef.Name)
	}

	if len(caData) == 0 {
		return nil, fmt.Errorf("CA data is empty in secret %s/%s key %s", namespace, secretRef.Name, secretRef.Key)
	}

	s.logger.Info("loaded CA certificate from secret",
		"secretName", secretRef.Name,
		"namespace", namespace,
		"key", secretRef.Key,
		"dataSize", len(caData),
	)

	return caData, nil
}
