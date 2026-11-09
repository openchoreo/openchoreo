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
	"strconv"
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
func buildHubbleFlowFilters(scope wirelogsScope) []*flow.FlowFilter {
	parts := []string{
		fmt.Sprintf("k8s:%s=%s", labels.LabelKeyNamespaceName, scope.namespace),
		fmt.Sprintf("k8s:%s=%s", labels.LabelKeyEnvironmentName, scope.environment),
	}
	if scope.project != "" {
		parts = append(parts, fmt.Sprintf("k8s:%s=%s", labels.LabelKeyProjectName, scope.project))
	}
	if scope.component != "" {
		parts = append(parts, fmt.Sprintf("k8s:%s=%s", labels.LabelKeyComponentName, scope.component))
		// The main line's pods carry no release line label.
		if !scope.allReleaseLines {
			if scope.releaseLine != "" {
				parts = append(parts, fmt.Sprintf("k8s:%s=%s", labels.LabelKeyReleaseLine, scope.releaseLine))
			} else {
				parts = append(parts, "!k8s:"+labels.LabelKeyReleaseLine)
			}
		}
	}
	selector := strings.Join(parts, ",")
	return []*flow.FlowFilter{
		{SourceLabel: []string{selector}},
		{DestinationLabel: []string{selector}},
	}
}

// newGetFlowsRequest assembles the live-tail flow request for the specified filters.
func newGetFlowsRequest(scope wirelogsScope) *observer.GetFlowsRequest {
	return &observer.GetFlowsRequest{
		Follow:    true,
		Whitelist: buildHubbleFlowFilters(scope),
	}
}

// hubbleFlowStream is the subset of observer.Observer_GetFlowsClient
type hubbleFlowStream interface {
	Recv() (*observer.GetFlowsResponse, error)
}

// openHubbleFlowStream dials hubble-relay over gRPC, opens a GetFlows server-stream,
// and returns the stream plus a closer that tears down the connection.
var openHubbleFlowStream = func(ctx context.Context, addr string, req *observer.GetFlowsRequest) (hubbleFlowStream, func(), error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial hubble-relay: %w", err)
	}
	client := observer.NewObserverClient(conn)
	stream, err := client.GetFlows(ctx, req)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("hubble GetFlows failed: %w", err)
	}
	return stream, func() { _ = conn.Close() }, nil
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
//
// parentCtx is the agent's message-loop context, so a websocket disconnect (or
// any cancel upstream) propagates into the gRPC Recv loop and tears the stream
// down without relying on the gateway delivering a Close chunk.
func (a *Agent) handleHubbleStreamInit(parentCtx context.Context, init *messaging.HTTPTunnelStreamInit) {
	logger := a.logger.With("requestID", init.RequestID, "target", "hubble")
	logger.Info("Received hubble stream init")

	params, err := url.ParseQuery(init.Query)
	if err != nil {
		logger.Warn("invalid hubble query", "error", err, "query", init.Query)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("invalid hubble query: %v", err))
		return
	}
	allReleaseLines, _ := strconv.ParseBool(params.Get("allReleaseLines"))
	scope := wirelogsScope{
		environment:     params.Get("environment"),
		namespace:       params.Get("namespace"),
		project:         params.Get("project"),
		component:       params.Get("component"),
		releaseLine:     params.Get("releaseLine"),
		allReleaseLines: allReleaseLines,
	}
	if scope.environment == "" || scope.namespace == "" {
		a.sendStreamClose(init.RequestID, "environment and namespace query params are required")
		return
	}
	if scope.releaseLine != "" && scope.allReleaseLines {
		a.sendStreamClose(init.RequestID, "set releaseLine or allReleaseLines, not both")
		return
	}

	ctx, cancel := context.WithCancel(parentCtx)
	session := &hubbleSession{
		requestID: init.RequestID,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	a.hubbleStreamsMu.Lock()
	if _, exists := a.hubbleStreams[init.RequestID]; exists {
		a.hubbleStreamsMu.Unlock()
		session.close()
		logger.Warn("duplicate hubble stream requestID; rejecting new session")
		a.sendStreamClose(init.RequestID, "duplicate hubble stream requestID")
		return
	}
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
		"component", scope.component, "project", scope.project, "environment", scope.environment, "namespace", scope.namespace,
		"releaseLine", scope.releaseLine, "allReleaseLines", scope.allReleaseLines,
	)

	stream, closer, err := openHubbleFlowStream(ctx, relayAddr, newGetFlowsRequest(scope))
	if err != nil {
		logger.Error("failed to open hubble flow stream", "error", err)
		a.sendStreamClose(init.RequestID, err.Error())
		return
	}
	defer closer()

	// Sentinel chunk so the gateway knows the stream is active
	sentinel := &messaging.HTTPTunnelStreamChunk{RequestID: init.RequestID, Data: []byte{}}
	if scope.component != "" && scope.releaseLine != "" {
		sentinel.Applied = []string{messaging.AppliedReleaseLine}
	}
	_ = a.sendStreamChunk(sentinel)

	logger.Info("Hubble flow stream started")

	marshalOpts := protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: false,
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
