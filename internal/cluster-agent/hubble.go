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
	"sync"

	"github.com/cilium/cilium/api/v1/flow"
	"github.com/cilium/cilium/api/v1/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// defaultHubbleRelayAddr is the in-cluster gRPC endpoint of the Hubble relay.
// Override with the HUBBLE_RELAY_ADDR env var on the cluster-agent.
const defaultHubbleRelayAddr = "hubble-relay.kube-system.svc.cluster.local:4245"

// buildHubbleFlowFilters returns the OR'd whitelist of FlowFilters used to follow
// Hubble flows for an OpenChoreo component. The list contains two filters so flows
// match when the component's pods are EITHER source OR destination.
//
// Each entry in FlowFilter.SourceLabel / DestinationLabel is treated as an
// independent label selector that is OR'd across the list; within a single
// selector, comma-separated k8s-style terms are AND'd (k8s.io/labels.Parse
// semantics). So all the component-identifying labels are joined into ONE
// comma-separated string per filter to ensure they must ALL match.
func buildHubbleFlowFilters(component, project, environment, controlPlaneNamespace string) []*flow.FlowFilter {
	selector := fmt.Sprintf(
		"k8s:openchoreo.dev/component=%s,k8s:openchoreo.dev/project=%s,k8s:openchoreo.dev/environment=%s,k8s:openchoreo.dev/namespace=%s",
		component, project, environment, controlPlaneNamespace,
	)
	return []*flow.FlowFilter{
		{SourceLabel: []string{selector}},
		{DestinationLabel: []string{selector}},
	}
}

// newGetFlowsRequest assembles the live-tail flow request for a component.
func newGetFlowsRequest(component, project, environment, controlPlaneNamespace string) *observer.GetFlowsRequest {
	return &observer.GetFlowsRequest{
		Follow:    true,
		Whitelist: buildHubbleFlowFilters(component, project, environment, controlPlaneNamespace),
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
// for the chunk's requestID. Hubble is server-streaming, so the only meaningful
// inbound chunk is the close signal. Returns true if the chunk belonged to a
// hubble session, letting the caller fall back to the exec router otherwise.
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

// hubbleRelayAddr returns the Hubble relay endpoint from the HUBBLE_RELAY_ADDR
// env var, falling back to the in-cluster default when unset. It is read lazily
// when the gateway invokes the hubble path, so wirelogs stay optional and the
// address is not a required cluster-agent startup config.
func hubbleRelayAddr() string {
	if addr := os.Getenv("HUBBLE_RELAY_ADDR"); addr != "" {
		return addr
	}
	return defaultHubbleRelayAddr
}

// handleHubbleStreamInit opens a server-streaming gRPC call to Hubble relay
// and forwards each flow event as an HTTPTunnelStreamChunk back to the gateway.
// Dispatched from Agent.handleHTTPTunnelStreamInit for Target == "hubble".
//
// Expected init.Query params: component, environment, namespace.
func (a *Agent) handleHubbleStreamInit(init *messaging.HTTPTunnelStreamInit) {
	logger := a.logger.With("requestID", init.RequestID, "target", "hubble")
	logger.Info("Received hubble stream init")

	params, err := url.ParseQuery(init.Query)
	if err != nil {
		logger.Warn("invalid hubble query", "error", err, "query", init.Query)
		a.sendStreamClose(init.RequestID, fmt.Sprintf("invalid hubble query: %v", err))
		return
	}
	component := params.Get("component")
	project := params.Get("project")
	environment := params.Get("environment")
	namespace := params.Get("namespace")
	if component == "" || project == "" || environment == "" || namespace == "" {
		a.sendStreamClose(init.RequestID, "component, project, environment, namespace query params are required")
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

	relayAddr := hubbleRelayAddr()
	logger = logger.With("hubbleRelay", relayAddr, "component", component, "project", project, "environment", environment)

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

	// Sentinel chunk so the gateway knows the stream is active (mirrors exec).
	a.sendStreamChunkRaw(init.RequestID, []byte{}, 0)

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
