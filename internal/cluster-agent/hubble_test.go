// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

func TestBuildHubbleFlowFilters_ORsSourceAndDestination(t *testing.T) {
	filters := buildHubbleFlowFilters("checkout", "shopfront", "development", "my-team")

	// Expect exactly two FlowFilters: one with SourceLabel, one with DestinationLabel,
	// so flows match when the component pods are EITHER source OR destination.
	require.Len(t, filters, 2)

	// Each FlowFilter.SourceLabel entry is its own k8s label selector that is
	// OR'd across the list. To require ALL labels match (AND semantics), the
	// labels must be joined into a single comma-separated selector string.
	// Each selector term is prefixed with `k8s:` so Hubble matches them against
	// the pod's Kubernetes labels (which it surfaces in the `k8s:` namespace).
	expected := "k8s:openchoreo.dev/component=checkout,k8s:openchoreo.dev/project=shopfront,k8s:openchoreo.dev/environment=development,k8s:openchoreo.dev/namespace=my-team"

	require.Len(t, filters[0].GetSourceLabel(), 1, "source filter must be a single comma-joined selector (AND), not multiple OR'd entries")
	assert.Equal(t, expected, filters[0].GetSourceLabel()[0])
	assert.Empty(t, filters[0].GetDestinationLabel(),
		"first filter must not constrain destination")

	require.Len(t, filters[1].GetDestinationLabel(), 1, "destination filter must be a single comma-joined selector")
	assert.Equal(t, expected, filters[1].GetDestinationLabel()[0])
	assert.Empty(t, filters[1].GetSourceLabel(),
		"second filter must not constrain source")
}

func TestNewGetFlowsRequest_LiveTail(t *testing.T) {
	req := newGetFlowsRequest("checkout", "shopfront", "development", "my-team")

	assert.True(t, req.GetFollow(), "wirelogs is a live tail; Follow must be true")
	assert.Zero(t, req.GetNumber(), "v1 does not replay history; Number must be 0")
	assert.Len(t, req.GetWhitelist(), 2, "request must carry both source and destination filters")
}

func TestHubbleRelayAddr_ErrorWhenUnset(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "")
	addr, err := hubbleRelayAddr()
	assert.ErrorIs(t, err, errors.New("HUBBLE_RELAY_ADDR env var is not set; it is required when configuring the Cilium module"))
	assert.Empty(t, addr)
}

func TestHubbleRelayAddr_FromEnv(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "hubble-relay.custom.svc:4245")
	addr, err := hubbleRelayAddr()
	assert.NoError(t, err)
	assert.Equal(t, "hubble-relay.custom.svc:4245", addr)
}

func TestHubbleSession_HandleChunkIsNoOp(t *testing.T) {
	// Hubble is server-streaming only; payload chunks from the API client are
	// ignored. Verify handleChunk does not panic and does not mutate state.
	s := &hubbleSession{requestID: "x", cancel: func() {}, done: make(chan struct{})}
	require.NotPanics(t, func() {
		s.handleChunk(nil)
	})
}

// TestAgent_HandleConnection_RoutesHubbleCloseWithNilData guards the gateway↔agent
// close protocol: the gateway signals close with IsClose set and no Data (see
// internal/cluster-gateway/wirelogs.go). The agent must still route it to the
// hubble session so the upstream gRPC stream is canceled. A nil-Data guard here
// previously dropped the close, leaking the session and misrouting the message
// as an HTTPTunnelRequest.
func TestAgent_HandleConnection_RoutesHubbleCloseWithNilData(t *testing.T) {
	closeChunk, err := json.Marshal(&messaging.HTTPTunnelStreamChunk{
		RequestID: "hubble-req-1",
		IsClose:   true, // Data intentionally left nil, mirroring the gateway
	})
	require.NoError(t, err)

	mock := &mockConnection{readMessages: [][]byte{closeChunk}}

	router := newTestRouter(t, map[string]*Route{})
	agent := newTestAgent(t, "ws://unused", router)
	agent.conn = mock
	agent.hubbleStreams = make(map[string]*hubbleSession)

	canceled := make(chan struct{})
	agent.hubbleStreams["hubble-req-1"] = &hubbleSession{
		requestID: "hubble-req-1",
		cancel:    func() { close(canceled) },
		done:      make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agent.handleConnection(ctx)

	// The close chunk must cancel the hubble session's gRPC context.
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("close chunk with nil Data did not cancel the hubble session")
	}

	// It must NOT be misrouted as an HTTPTunnelRequest (which would write a response).
	assert.Empty(t, mock.getWrittenMessages(),
		"close chunk must not be handled as an HTTP tunnel request")
}

func TestHubbleSession_CloseIsIdempotent(t *testing.T) {
	cancelCalls := 0
	s := &hubbleSession{
		requestID: "x",
		cancel:    func() { cancelCalls++ },
		done:      make(chan struct{}),
	}
	s.close()
	s.close() // second close must not panic on closed channel or recall cancel
	assert.Equal(t, 1, cancelCalls)
}
