// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/cilium/cilium/api/v1/flow"
	"github.com/cilium/cilium/api/v1/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// buildHubbleFlowFilters returns the OR'd whitelist of FlowFilters used to follow
// Hubble flows for components in the specified environment. The list contains two filters
// so flows match when the matching pods are EITHER source OR destination.
func buildHubbleFlowFilters(component, project, environment, namespace string) []*flow.FlowFilter {
	parts := []string{
		fmt.Sprintf("k8s:%s=%s", labels.LabelKeyNamespaceName, namespace),
		fmt.Sprintf("k8s:%s=%s", labels.LabelKeyEnvironmentName, environment),
	}
	if project != "" {
		parts = append(parts, fmt.Sprintf("k8s:%s=%s", labels.LabelKeyProjectName, project))
	}
	if component != "" {
		parts = append(parts, fmt.Sprintf("k8s:%s=%s", labels.LabelKeyComponentName, component))
	}
	selector := strings.Join(parts, ",")
	return []*flow.FlowFilter{
		{SourceLabel: []string{selector}},
		{DestinationLabel: []string{selector}},
	}
}

// newGetFlowsRequest assembles the live-tail flow request for the specified filters.
func newGetFlowsRequest(component, project, environment, namespace string) *observer.GetFlowsRequest {
	return &observer.GetFlowsRequest{
		Follow:    true,
		Whitelist: buildHubbleFlowFilters(component, project, environment, namespace),
	}
}

// hubbleSession is a server-streaming session that forwards Hubble flow events
// from hubble-relay to the gateway, framed as HTTPTunnelStreamChunks.
type hubbleSession struct {
	requestID string
	cancel    context.CancelFunc
	done      chan struct{}
	once      sync.Once
}

// handleChunk: Hubble is server-streaming only — the API client never sends
// payload chunks. Close is handled in Agent.routeHubbleChunk.
func (s *hubbleSession) handleChunk(_ *messaging.HTTPTunnelStreamChunk) {}

func (s *hubbleSession) close() {
	s.once.Do(func() {
		close(s.done)
		s.cancel()
	})
}

// routeHubbleChunk delivers an inbound chunk to its hubble session, if one exists
// for the chunk's requestID.
func (a *Agent) routeHubbleChunk(chunk *messaging.HTTPTunnelStreamChunk) bool {
	a.hubbleStreamsMu.Lock()
	session, ok := a.hubbleStreams[chunk.RequestID]
	a.hubbleStreamsMu.Unlock()

	if !ok {
		return false
	}

	if chunk.IsClose {
		session.close()
		return true
	}

	session.handleChunk(chunk)
	return true
}

func hubbleRelayAddr() (string, error) {
	addr := os.Getenv("HUBBLE_RELAY_ADDR")
	if addr == "" {
		return "", errors.New("HUBBLE_RELAY_ADDR env var is not set; it is required when configuring the Cilium module")
	}
	return addr, nil
}

// handleHubbleStreamInit opens a server-streaming gRPC call to Hubble relay
// and forwards each flow event as an HTTPTunnelStreamChunk back to the gateway.
// Dispatched from Agent.handleHTTPTunnelStreamInit for Target == "hubble".
func (a *Agent) handleHubbleStreamInit(init *messaging.HTTPTunnelStreamInit) {
	logger := a.logger.With("requestID", init.RequestID, "target", "hubble")
	logger.Info("Received hubble stream init")

	params, err := url.ParseQuery(init.Query)
	if err != nil {
		logger.Warn("invalid hubble query", "error", err, "query", init.Query)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("invalid hubble query: %v", err))
		return
	}
	environment := params.Get("environment")
	namespace := params.Get("namespace")
	project := params.Get("project")
	component := params.Get("component")
	if environment == "" || namespace == "" {
		a.sendStreamClose(init.RequestID, "environment and namespace query params are required")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	session := &hubbleSession{
		requestID: init.RequestID,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	a.hubbleStreamsMu.Lock()
	a.hubbleStreams[init.RequestID] = session
	a.hubbleStreamsMu.Unlock()

	defer func() {
		session.close()
		a.hubbleStreamsMu.Lock()
		delete(a.hubbleStreams, init.RequestID)
		a.hubbleStreamsMu.Unlock()
	}()

	relayAddr, err := hubbleRelayAddr()
	if err != nil {
		logger.Error("hubble relay address not configured", "error", err)
		a.sendStreamClose(init.RequestID, err.Error())
		return
	}
	logger = logger.With(
		"hubbleRelay", relayAddr,
		"component", component, "project", project, "environment", environment, "namespace", namespace,
	)

	conn, err := grpc.NewClient(
		relayAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Error("failed to create hubble-relay client", "error", err)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("failed to dial hubble-relay: %v", err))
		return
	}
	defer conn.Close()

	client := observer.NewObserverClient(conn)
	stream, err := client.GetFlows(ctx, newGetFlowsRequest(component, project, environment, namespace))
	if err != nil {
		logger.Error("GetFlows failed", "error", err)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("hubble GetFlows failed: %v", err))
		return
	}

	// Sentinel chunk so the gateway knows the stream is active
	a.sendStreamChunkRaw(init.RequestID, []byte{}, 0)

	logger.Info("Hubble flow stream started")

	marshalOpts := protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
	}
	scope := wirelogsScope{
		environment: environment,
		namespace:   namespace,
		project:     project,
		component:   component,
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				logger.Info("hubble stream closed")
			} else {
				logger.Warn("hubble stream ended with error", "error", err)
			}
			break
		}

		if !redactFlowResponse(resp, scope) {
			continue
		}

		data, err := marshalOpts.Marshal(resp)
		if err != nil {
			logger.Warn("failed to marshal flow response", "error", err)
			continue
		}

		chunk := &messaging.HTTPTunnelStreamChunk{
			RequestID: init.RequestID,
			Data:      data,
		}
		if err := a.sendStreamChunk(chunk); err != nil {
			logger.Warn("failed to forward flow chunk; closing stream", "error", err)
			return
		}
	}

	logger.Info("Hubble flow stream completed")
	a.sendStreamClose(init.RequestID, "")
}
