// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

func TestBuildHubbleFlowFilters_ORsSourceAndDestination(t *testing.T) {
	filters := buildHubbleFlowFilters("checkout", "shopfront", "development", "my-team")

	// Two filters (source-only, destination-only) so flows match when the
	// component is EITHER side. Labels within a filter are comma-joined into one
	// selector so they AND together — separate entries would OR.
	require.Len(t, filters, 2)
	expected := "k8s:openchoreo.dev/namespace=my-team,k8s:openchoreo.dev/environment=development,k8s:openchoreo.dev/project=shopfront,k8s:openchoreo.dev/component=checkout"

	require.Len(t, filters[0].GetSourceLabel(), 1, "must be a single comma-joined selector (AND), not multiple OR'd entries")
	assert.Equal(t, expected, filters[0].GetSourceLabel()[0])
	assert.Empty(t, filters[0].GetDestinationLabel())

	require.Len(t, filters[1].GetDestinationLabel(), 1)
	assert.Equal(t, expected, filters[1].GetDestinationLabel()[0])
	assert.Empty(t, filters[1].GetSourceLabel())
}

func TestBuildHubbleFlowFilters_EnvironmentWide(t *testing.T) {
	filters := buildHubbleFlowFilters("", "", "development", "my-team")

	require.Len(t, filters, 2)
	expected := "k8s:openchoreo.dev/namespace=my-team,k8s:openchoreo.dev/environment=development"

	require.Len(t, filters[0].GetSourceLabel(), 1)
	assert.Equal(t, expected, filters[0].GetSourceLabel()[0])
	assert.Empty(t, filters[0].GetDestinationLabel())

	require.Len(t, filters[1].GetDestinationLabel(), 1)
	assert.Equal(t, expected, filters[1].GetDestinationLabel()[0])
}

func TestBuildHubbleFlowFilters_ProjectOnly(t *testing.T) {
	filters := buildHubbleFlowFilters("", "shopfront", "development", "my-team")

	require.Len(t, filters, 2)
	expected := "k8s:openchoreo.dev/namespace=my-team,k8s:openchoreo.dev/environment=development,k8s:openchoreo.dev/project=shopfront"
	assert.Equal(t, expected, filters[0].GetSourceLabel()[0])
	assert.Equal(t, expected, filters[1].GetDestinationLabel()[0])
}

func TestBuildHubbleFlowFilters_ComponentWithoutProject(t *testing.T) {
	filters := buildHubbleFlowFilters("checkout", "", "development", "my-team")

	require.Len(t, filters, 2)
	expected := "k8s:openchoreo.dev/namespace=my-team,k8s:openchoreo.dev/environment=development,k8s:openchoreo.dev/component=checkout"
	assert.Equal(t, expected, filters[0].GetSourceLabel()[0])
	assert.Equal(t, expected, filters[1].GetDestinationLabel()[0])
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HUBBLE_RELAY_ADDR")
	assert.Empty(t, addr)
}

func TestHubbleRelayAddr_FromEnv(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "hubble-relay.custom.svc:4245")
	addr, err := hubbleRelayAddr()
	assert.NoError(t, err)
	assert.Equal(t, "hubble-relay.custom.svc:4245", addr)
}

func TestHubbleSession_HandleChunkIsNoOp(t *testing.T) {
	// Hubble is server-streaming only; client-side payload chunks are ignored.
	s := &hubbleSession{requestID: "x", cancel: func() {}, done: make(chan struct{})}
	require.NotPanics(t, func() {
		s.handleChunk(nil)
	})
}

// Regression: the gateway signals close with IsClose set and no Data. A nil-Data
// guard in handleConnection previously dropped these, leaking the session and
// misrouting the message as an HTTPTunnelRequest.
func TestAgent_HandleConnection_RoutesHubbleCloseWithNilData(t *testing.T) {
	closeChunk, err := json.Marshal(&messaging.HTTPTunnelStreamChunk{
		RequestID: "hubble-req-1",
		IsClose:   true,
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

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("close chunk with nil Data did not cancel the hubble session")
	}

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
	s.close()
	assert.Equal(t, 1, cancelCalls)
}
