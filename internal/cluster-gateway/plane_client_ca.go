// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"
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

// workflowPlaneInfo extracts plane information from a WorkflowPlane CR
// Returns the extracted info and true if the CR should be processed
func workflowPlaneInfo(obj client.Object) (planeInfo, bool) {
	wp, ok := obj.(*openchoreov1alpha1.WorkflowPlane)
	if !ok {
		return planeInfo{}, false
	}

	planeID := wp.Spec.PlaneID
	if planeID == "" {
		planeID = wp.Name
	}

	return planeInfo{
		name:      wp.Name,
		namespace: wp.Namespace,
		planeID:   planeID,
		agent:     &wp.Spec.ClusterAgent,
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

// clusterWorkflowPlaneInfo extracts plane information from a ClusterWorkflowPlane CR
// Returns the extracted info and true if the CR should be processed
func clusterWorkflowPlaneInfo(obj client.Object) (planeInfo, bool) {
	cwp, ok := obj.(*openchoreov1alpha1.ClusterWorkflowPlane)
	if !ok {
		return planeInfo{}, false
	}

	return planeInfo{
		name:      cwp.Name,
		namespace: "", // Cluster-scoped - no namespace
		planeID:   cwp.Spec.PlaneID,
		agent:     &cwp.Spec.ClusterAgent,
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

// connectLookupTimeout bounds the client CA lookup of one agent connect. The
// agent gives up on its handshake after 10s and starts the connect again, so
// the lookup takes half of that and leaves the rest for the handshake itself,
// which also slows down when the pod is short of CPU.
var connectLookupTimeout = 5 * time.Second

// caReadConcurrency is how many CA Secrets one agent connect reads at a time.
const caReadConcurrency = 16

// extractPlaneClientCAs is a generic helper that extracts client CAs from a list of plane CRs
// It eliminates code duplication across DataPlane, WorkflowPlane, and ObservabilityPlane processing
func (s *Server) extractPlaneClientCAs(
	ctx context.Context,
	planeType string,
	planeID string,
	list client.ObjectList,
	extractInfo func(client.Object) (planeInfo, bool),
) (cas map[string][]byte, unread []string, err error) {
	if err := s.planes().List(ctx, list); err != nil {
		return nil, nil, fmt.Errorf("failed to list %s: %w", planeType, err)
	}

	// Extract items from the list
	// All plane list types have an Items field that implements client.Object
	items, err := extractListItems(list)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract items from list: %w", err)
	}

	var crs []planeInfo
	for _, item := range items {
		info, ok := extractInfo(item)
		if !ok {
			continue // Skip items that can't be extracted (shouldn't happen)
		}

		if info.planeID != planeID {
			continue
		}
		crs = append(crs, info)
	}

	cas, unread = s.clientCAsForConnect(ctx, planeType, crs)
	return cas, unread, nil
}

// clientCAsForConnect returns the client CA of each CR, keyed "namespace/name",
// for an agent connect to verify against. A CA held in a Secret is taken from
// the CA cache when it was read recently, and otherwise read from the API
// server, caReadConcurrency at a time.
//
// A CR whose CA cannot be read is left out. A CR still unread when ctx ends is
// left out too and returned in unread: the connect then goes ahead with the
// CRs it has, instead of failing for all of them, and the caller authorizes the
// unread ones once the connection is registered.
func (s *Server) clientCAsForConnect(
	ctx context.Context, planeType string, crs []planeInfo,
) (cas map[string][]byte, unread []string) {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		slots = make(chan struct{}, caReadConcurrency)
	)
	cas = make(map[string][]byte, len(crs))

	resolve := func(info planeInfo) {
		crKey := fmt.Sprintf("%s/%s", info.namespace, info.name)
		caData, err := s.extractCAFromPlane(ctx, info.agent, info.namespace)

		mu.Lock()
		defer mu.Unlock()
		switch {
		case err == nil:
			cas[crKey] = caData
		case ctx.Err() != nil:
			unread = append(unread, crKey)
		default:
			s.logger.Warn("failed to extract CA from plane CR",
				"planeType", planeType,
				"namespace", info.namespace,
				"name", info.name,
				"error", err,
			)
		}
	}

	for _, info := range crs {
		key, inSecret := secretCARef(info)
		if !inSecret {
			resolve(info) // An inline CA, or none: nothing to read.
			continue
		}
		crKey := fmt.Sprintf("%s/%s", info.namespace, info.name)
		if caData, cached := s.cas().get(key); cached {
			mu.Lock()
			cas[crKey] = caData
			mu.Unlock()
			continue
		}
		if ctx.Err() != nil {
			mu.Lock()
			unread = append(unread, crKey)
			mu.Unlock()
			continue
		}

		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			resolve(info)
		}()
	}
	wg.Wait()

	if len(unread) > 0 {
		s.logger.Warn("agent connect ran out of time reading client CAs; the CRs left out are authorized after the connect",
			"planeType", planeType,
			"unreadCRs", len(unread),
			"totalCRs", len(crs),
		)
	}
	return cas, unread
}

// secretCARef returns the Secret key a CR's client CA is read from, when the CA
// is held in a Secret and not inline.
func secretCARef(info planeInfo) (secretCAKey, bool) {
	if info.agent == nil || info.agent.ClientCA.Value != "" || info.agent.ClientCA.SecretKeyRef == nil {
		return secretCAKey{}, false
	}
	return secretCAKeyFor(info.agent.ClientCA.SecretKeyRef, info.namespace), true
}

// extractListItems extracts the Items slice from a client.ObjectList
// This handles the type assertion for DataPlaneList, WorkflowPlaneList, ObservabilityPlaneList
func extractListItems(list client.ObjectList) ([]client.Object, error) {
	switch v := list.(type) {
	case *openchoreov1alpha1.DataPlaneList:
		items := make([]client.Object, len(v.Items))
		for i := range v.Items {
			items[i] = &v.Items[i]
		}
		return items, nil
	case *openchoreov1alpha1.WorkflowPlaneList:
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
	case *openchoreov1alpha1.ClusterWorkflowPlaneList:
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
func (s *Server) getAllPlaneClientCAs(planeType, planeID string) (cas map[string][]byte, unread []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectLookupTimeout)
	defer cancel()

	result := make(map[string][]byte)

	// Query both namespace-scoped and cluster-scoped CRs for each plane type
	switch planeType {
	case planeTypeDataPlane:
		// Namespace-scoped DataPlane
		nsResult, nsUnread, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.DataPlaneList{}, dataPlaneInfo)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range nsResult {
			result[k] = v
		}
		unread = append(unread, nsUnread...)
		// Cluster-scoped ClusterDataPlane
		clusterResult, clusterUnread, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ClusterDataPlaneList{}, clusterDataPlaneInfo)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range clusterResult {
			result[k] = v
		}
		unread = append(unread, clusterUnread...)

	case planeTypeWorkflowPlane:
		// Namespace-scoped WorkflowPlane
		nsResult, nsUnread, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.WorkflowPlaneList{}, workflowPlaneInfo)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range nsResult {
			result[k] = v
		}
		unread = append(unread, nsUnread...)
		// Cluster-scoped ClusterWorkflowPlane
		clusterResult, clusterUnread, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ClusterWorkflowPlaneList{}, clusterWorkflowPlaneInfo)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range clusterResult {
			result[k] = v
		}
		unread = append(unread, clusterUnread...)

	case planeTypeObservabilityPlane:
		// Namespace-scoped ObservabilityPlane
		nsResult, nsUnread, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ObservabilityPlaneList{}, obsPlaneInfo)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range nsResult {
			result[k] = v
		}
		unread = append(unread, nsUnread...)
		// Cluster-scoped ClusterObservabilityPlane
		clusterResult, clusterUnread, err := s.extractPlaneClientCAs(ctx, planeType, planeID,
			&openchoreov1alpha1.ClusterObservabilityPlaneList{}, clusterObsPlaneInfo)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range clusterResult {
			result[k] = v
		}
		unread = append(unread, clusterUnread...)

	default:
		return nil, nil, fmt.Errorf("unsupported plane type: %s", planeType)
	}

	s.logger.Info("retrieved client CAs from CRs",
		"planeType", planeType,
		"planeID", planeID,
		"totalCRs", len(result),
	)

	if len(result) > 0 {
		for crKey := range result {
			s.logger.Debug("discovered CR for plane",
				"planeType", planeType,
				"planeID", planeID,
				"cr", crKey,
			)
		}
	} else {
		s.logger.Warn("no CRs found for connecting agent",
			"planeType", planeType,
			"planeID", planeID,
			"note", "the connection will be rejected: with no CR there is no CA to verify the agent against",
		)
	}

	return result, unread, nil
}

// planeCRObject returns an empty CR of the kind that a plane type and a CR
// namespace identify, with the function that extracts its plane info.
// An empty namespace selects the cluster-scoped kind.
func planeCRObject(planeType, namespace string) (client.Object, func(client.Object) (planeInfo, bool), error) {
	clusterScoped := namespace == ""
	switch planeType {
	case planeTypeDataPlane:
		if clusterScoped {
			return &openchoreov1alpha1.ClusterDataPlane{}, clusterDataPlaneInfo, nil
		}
		return &openchoreov1alpha1.DataPlane{}, dataPlaneInfo, nil
	case planeTypeWorkflowPlane:
		if clusterScoped {
			return &openchoreov1alpha1.ClusterWorkflowPlane{}, clusterWorkflowPlaneInfo, nil
		}
		return &openchoreov1alpha1.WorkflowPlane{}, workflowPlaneInfo, nil
	case planeTypeObservabilityPlane:
		if clusterScoped {
			return &openchoreov1alpha1.ClusterObservabilityPlane{}, clusterObsPlaneInfo, nil
		}
		return &openchoreov1alpha1.ObservabilityPlane{}, obsPlaneInfo, nil
	default:
		return nil, nil, fmt.Errorf("unsupported plane type: %s", planeType)
	}
}

// extractCAFromPlane extracts CA data from a plane's ClusterAgent configuration
func (s *Server) extractCAFromPlane(
	ctx context.Context, clusterAgent *openchoreov1alpha1.ClusterAgentConfig, namespace string,
) ([]byte, error) {
	if clusterAgent == nil {
		return nil, nil
	}
	return s.extractCADataWithNamespace(ctx, &clusterAgent.ClientCA, namespace)
}

// extractCAData extracts CA certificate data from ValueFrom configuration
// planeNamespace is used as default namespace for SecretRef if not specified
func (s *Server) extractCADataWithNamespace(
	ctx context.Context, valueFrom *openchoreov1alpha1.ValueFrom, planeNamespace string,
) ([]byte, error) {
	if valueFrom.Value != "" {
		return []byte(valueFrom.Value), nil
	}

	if valueFrom.SecretKeyRef != nil {
		return s.extractCAFromSecret(ctx, valueFrom.SecretKeyRef, planeNamespace)
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
//
// It always reads the API server. What it reads is recorded in the CA cache for
// agent connects to reuse, and a failed read removes the entry, so the cache
// never holds a CA that a later read has contradicted.
func (s *Server) extractCAFromSecret(
	ctx context.Context, secretRef *openchoreov1alpha1.SecretKeyReference, planeNamespace string,
) ([]byte, error) {
	if secretRef.Name == "" {
		return nil, fmt.Errorf("secret name is required")
	}

	if secretRef.Key == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	key := secretCAKeyFor(secretRef, planeNamespace)
	readAt := s.cas().now()
	caData, err := s.readCAFromSecret(ctx, key)
	if err != nil {
		s.cas().drop(key)
		return nil, err
	}
	s.cas().put(key, caData, readAt)
	return caData, nil
}

// readCAFromSecret reads one CA bundle from its Secret.
func (s *Server) readCAFromSecret(ctx context.Context, key secretCAKey) ([]byte, error) {
	s.logger.Debug("loading CA certificate from secret",
		"secretName", key.name,
		"namespace", key.namespace,
		"key", key.key,
	)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Name:      key.name,
		Namespace: key.namespace,
	}

	if err := s.k8sClient.Get(ctx, secretKey, &secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", key.namespace, key.name, err)
	}

	caData, ok := secret.Data[key.key]
	if !ok {
		return nil, fmt.Errorf("key %s not found in secret %s/%s", key.key, key.namespace, key.name)
	}

	if len(caData) == 0 {
		return nil, fmt.Errorf("CA data is empty in secret %s/%s key %s", key.namespace, key.name, key.key)
	}

	s.logger.Debug("loaded CA certificate from secret",
		"secretName", key.name,
		"namespace", key.namespace,
		"key", key.key,
		"dataSize", len(caData),
	)

	return caData, nil
}

// planeCRTerminating reports whether a plane CR is being deleted. A failed
// lookup is returned as an error, not as "not terminating": a caller that took
// it for the latter could re-grant a CR it has already revoked.
func (s *Server) planeCRTerminating(ctx context.Context, planeType, namespace, name string) (bool, error) {
	obj, err := s.getPlaneCR(ctx, planeType, namespace, name)
	if err != nil {
		return false, fmt.Errorf("failed to look up %s %s/%s: %w", planeType, namespace, name, err)
	}
	return !obj.GetDeletionTimestamp().IsZero(), nil
}

// getPlaneCR reads one plane CR. A cluster-scoped CR has an empty namespace.
func (s *Server) getPlaneCR(ctx context.Context, planeType, namespace, name string) (client.Object, error) {
	var obj client.Object
	switch {
	case planeType == planeTypeDataPlane && namespace != "":
		obj = &openchoreov1alpha1.DataPlane{}
	case planeType == planeTypeDataPlane:
		obj = &openchoreov1alpha1.ClusterDataPlane{}
	case planeType == planeTypeWorkflowPlane && namespace != "":
		obj = &openchoreov1alpha1.WorkflowPlane{}
	case planeType == planeTypeWorkflowPlane:
		obj = &openchoreov1alpha1.ClusterWorkflowPlane{}
	case planeType == planeTypeObservabilityPlane && namespace != "":
		obj = &openchoreov1alpha1.ObservabilityPlane{}
	case planeType == planeTypeObservabilityPlane:
		obj = &openchoreov1alpha1.ClusterObservabilityPlane{}
	default:
		return nil, fmt.Errorf("unknown plane type %q", planeType)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// existingPlaneCRs lists the CRs of one plane type, namespaced and
// cluster-scoped, and returns the effective plane ID of each, keyed
// "namespace/name". The namespace of a cluster-scoped CR is empty. The lists
// come from planeReader, so they can trail the API server: a caller must look a
// CR up before acting on its absence.
func (s *Server) existingPlaneCRs(ctx context.Context, planeType string) (map[string]string, error) {
	type planeList struct {
		list client.ObjectList
		info func(client.Object) (planeInfo, bool)
	}
	var lists []planeList
	switch planeType {
	case planeTypeDataPlane:
		lists = []planeList{{&openchoreov1alpha1.DataPlaneList{}, dataPlaneInfo}, {&openchoreov1alpha1.ClusterDataPlaneList{}, clusterDataPlaneInfo}}
	case planeTypeWorkflowPlane:
		lists = []planeList{{&openchoreov1alpha1.WorkflowPlaneList{}, workflowPlaneInfo}, {&openchoreov1alpha1.ClusterWorkflowPlaneList{}, clusterWorkflowPlaneInfo}}
	case planeTypeObservabilityPlane:
		lists = []planeList{{&openchoreov1alpha1.ObservabilityPlaneList{}, obsPlaneInfo}, {&openchoreov1alpha1.ClusterObservabilityPlaneList{}, clusterObsPlaneInfo}}
	default:
		return nil, fmt.Errorf("unsupported plane type: %s", planeType)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	existing := make(map[string]string)
	for _, l := range lists {
		if err := s.planes().List(ctx, l.list); err != nil {
			return nil, fmt.Errorf("failed to list %T: %w", l.list, err)
		}
		items, err := extractListItems(l.list)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if info, ok := l.info(item); ok {
				existing[item.GetNamespace()+"/"+item.GetName()] = info.planeID
			}
		}
	}
	return existing, nil
}

// planeCRPlaneID returns the effective plane ID of a plane CR: a DataPlane
// without one uses its name. It returns "" for an object that is not a plane CR.
func planeCRPlaneID(obj client.Object) string {
	for _, info := range []func(client.Object) (planeInfo, bool){
		dataPlaneInfo, clusterDataPlaneInfo,
		workflowPlaneInfo, clusterWorkflowPlaneInfo,
		obsPlaneInfo, clusterObsPlaneInfo,
	} {
		if i, ok := info(obj); ok {
			return i.planeID
		}
	}
	return ""
}
