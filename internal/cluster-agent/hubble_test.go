// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cilium/cilium/api/v1/flow"
	"github.com/cilium/cilium/api/v1/observer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// fakeFlowStream is a test double for hubbleFlowStream that yields canned
// responses in order, then an optional terminal error (defaults to io.EOF).
type fakeFlowStream struct {
	responses []*observer.GetFlowsResponse
	idx       int
	endErr    error
}

func (f *fakeFlowStream) Recv() (*observer.GetFlowsResponse, error) {
	if f.idx >= len(f.responses) {
		if f.endErr != nil {
			return nil, f.endErr
		}
		return nil, io.EOF
	}
	r := f.responses[f.idx]
	f.idx++
	return r, nil
}

// stubOpenHubbleFlowStream replaces the package-level dialer with a function
// returning the supplied stream (or err). The original is restored via t.Cleanup.
func stubOpenHubbleFlowStream(t *testing.T, stream hubbleFlowStream, err error) {
	t.Helper()
	prev := openHubbleFlowStream
	closerCalls := 0
	openHubbleFlowStream = func(_ context.Context, _ string, _ *observer.GetFlowsRequest) (hubbleFlowStream, func(), error) {
		if err != nil {
			return nil, nil, err
		}
		return stream, func() { closerCalls++ }, nil
	}
	t.Cleanup(func() {
		openHubbleFlowStream = prev
		// Closer must run exactly once on the happy/error-after-open paths.
		if err == nil && closerCalls != 1 {
			t.Errorf("closer expected to run exactly once, got %d", closerCalls)
		}
	})
}

// inScopeFlow returns a GetFlowsResponse whose Source labels mark it as
// in-scope for the test scope, so redactFlowResponse keeps the chunk.
func inScopeFlow(t *testing.T) *observer.GetFlowsResponse {
	t.Helper()
	return &observer.GetFlowsResponse{
		ResponseTypes: &observer.GetFlowsResponse_Flow{
			Flow: &flow.Flow{
				Source: &flow.Endpoint{
					Namespace: "dp-team-1",
					Labels: []string{
						"k8s:openchoreo.dev/namespace=team-1",
						"k8s:openchoreo.dev/environment=development",
					},
				},
			},
		},
	}
}

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

// newHubbleTestAgent wires a fresh Agent + mockConnection together with the
// hubbleStreams map initialized, which newTestAgent leaves nil.
func newHubbleTestAgent(t *testing.T) (*Agent, *mockConnection) {
	t.Helper()
	agent := newTestAgent(t, "ws://unused", nil)
	mock := &mockConnection{}
	agent.conn = mock
	agent.hubbleStreams = make(map[string]*hubbleSession)
	return agent, mock
}

// decodeChunks unmarshals every captured websocket frame as an
// HTTPTunnelStreamChunk. The handler only writes chunk messages, so a failure
// here means the agent emitted an unexpected frame type.
func decodeChunks(t *testing.T, raw [][]byte) []messaging.HTTPTunnelStreamChunk {
	t.Helper()
	out := make([]messaging.HTTPTunnelStreamChunk, 0, len(raw))
	for _, b := range raw {
		var c messaging.HTTPTunnelStreamChunk
		require.NoError(t, json.Unmarshal(b, &c))
		out = append(out, c)
	}
	return out
}

func TestAgent_HandleHubbleStreamInit_InvalidQuery(t *testing.T) {
	agent, mock := newHubbleTestAgent(t)

	// "%zz" is an invalid percent-escape — url.ParseQuery rejects it.
	agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
		RequestID: "req-1",
		Target:    "hubble",
		Query:     "%zz",
	})

	chunks := decodeChunks(t, mock.getWrittenMessages())
	require.Len(t, chunks, 1, "should emit a single close chunk")
	assert.Equal(t, "req-1", chunks[0].RequestID)
	assert.True(t, chunks[0].IsClose)
	assert.Contains(t, string(chunks[0].Data), "invalid hubble query")
	assert.Empty(t, agent.hubbleStreams, "session must be removed before any registration")
}

func TestAgent_HandleHubbleStreamInit_MissingRequiredParams(t *testing.T) {
	agent, mock := newHubbleTestAgent(t)

	// "environment" is set but "namespace" is missing.
	agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
		RequestID: "req-2",
		Target:    "hubble",
		Query:     "environment=development",
	})

	chunks := decodeChunks(t, mock.getWrittenMessages())
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsClose)
	assert.Contains(t, string(chunks[0].Data), "namespace")
}

func TestAgent_HandleHubbleStreamInit_RelayAddrUnset(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "")
	agent, mock := newHubbleTestAgent(t)

	agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
		RequestID: "req-3",
		Target:    "hubble",
		Query:     "environment=development&namespace=team-1",
	})

	chunks := decodeChunks(t, mock.getWrittenMessages())
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsClose)
	assert.Contains(t, string(chunks[0].Data), "HUBBLE_RELAY_ADDR")
	assert.Empty(t, agent.hubbleStreams, "session must be cleaned up on early failure")
}

func TestAgent_HandleHubbleStreamInit_OpenStreamError(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "hubble-relay.test.svc:4245")
	stubOpenHubbleFlowStream(t, nil, errors.New("dial refused"))

	agent, mock := newHubbleTestAgent(t)
	agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
		RequestID: "req-4",
		Target:    "hubble",
		Query:     "environment=development&namespace=team-1",
	})

	chunks := decodeChunks(t, mock.getWrittenMessages())
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsClose)
	assert.Contains(t, string(chunks[0].Data), "dial refused")
	assert.Empty(t, agent.hubbleStreams)
}

func TestAgent_HandleHubbleStreamInit_HappyPath(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "hubble-relay.test.svc:4245")

	stream := &fakeFlowStream{
		responses: []*observer.GetFlowsResponse{inScopeFlow(t), inScopeFlow(t)},
		// endErr unset → second Recv after responses returns io.EOF and the
		// handler exits its loop cleanly.
	}
	stubOpenHubbleFlowStream(t, stream, nil)

	agent, mock := newHubbleTestAgent(t)
	agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
		RequestID: "req-5",
		Target:    "hubble",
		Query:     "environment=development&namespace=team-1",
	})

	chunks := decodeChunks(t, mock.getWrittenMessages())
	// Expected: 1 sentinel (empty Data) + 2 flow chunks + 1 final close.
	require.Len(t, chunks, 4, "sentinel + 2 flows + close")

	assert.False(t, chunks[0].IsClose)
	assert.Empty(t, chunks[0].Data, "first frame is the activation sentinel")

	for i, c := range chunks[1:3] {
		assert.False(t, c.IsClose, "flow chunk %d must not be a close", i)
		assert.NotEmpty(t, c.Data, "flow chunk %d must carry payload", i)
		assert.Contains(t, string(c.Data), "openchoreo.dev/environment=development",
			"protojson payload should carry the in-scope label")
	}

	assert.True(t, chunks[3].IsClose, "final frame must be the EOF close marker")
	assert.Empty(t, chunks[3].Data, "clean EOF close carries no error message")

	assert.Empty(t, agent.hubbleStreams, "session must be deregistered on stream completion")
}

// ctxBlockingFlowStream blocks Recv until its ctx is cancelled, then returns
// the ctx error. Used to drive the parent-cancellation regression test.
type ctxBlockingFlowStream struct {
	ctx context.Context
}

func (s *ctxBlockingFlowStream) Recv() (*observer.GetFlowsResponse, error) {
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

// Regression: canceling the parent ctx (e.g. websocket disconnect) must
// propagate into the gRPC Recv loop so the hubble session tears down without
// waiting for the gateway to deliver a Close chunk.
func TestAgent_HandleHubbleStreamInit_ParentCtxCancelStopsStream(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "hubble-relay.test.svc:4245")

	// Capture the ctx the handler hands to the dialer and route it into a
	// stream that blocks until cancelled — so we can verify the parent ctx
	// reaches the Recv loop end-to-end.
	prev := openHubbleFlowStream
	openHubbleFlowStream = func(ctx context.Context, _ string, _ *observer.GetFlowsRequest) (hubbleFlowStream, func(), error) {
		return &ctxBlockingFlowStream{ctx: ctx}, func() {}, nil
	}
	t.Cleanup(func() { openHubbleFlowStream = prev })

	agent, mock := newHubbleTestAgent(t)

	parentCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		agent.handleHubbleStreamInit(parentCtx, &messaging.HTTPTunnelStreamInit{
			RequestID: "req-ctx",
			Target:    "hubble",
			Query:     "environment=development&namespace=team-1",
		})
		close(done)
	}()

	// Wait for the sentinel chunk so we know the handler has entered the Recv loop.
	require.Eventually(t, func() bool {
		return len(mock.getWrittenMessages()) >= 1
	}, time.Second, 5*time.Millisecond, "sentinel chunk should fire once stream is active")

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("parent ctx cancel did not unblock handleHubbleStreamInit")
	}

	chunks := decodeChunks(t, mock.getWrittenMessages())
	require.GreaterOrEqual(t, len(chunks), 2, "expected at least sentinel + final close")
	last := chunks[len(chunks)-1]
	assert.True(t, last.IsClose, "final frame must be the close marker")
	assert.Empty(t, agent.hubbleStreams, "session must be deregistered after ctx cancel")
}

// Regression: a second init for an already-active RequestID must NOT overwrite
// or evict the existing session. The guard rejects the duplicate with a
// close chunk and leaves the original session intact.
func TestAgent_HandleHubbleStreamInit_DuplicateRequestIDRejected(t *testing.T) {
	agent, mock := newHubbleTestAgent(t)

	cancelCalls := 0
	existing := &hubbleSession{
		requestID: "req-dup",
		cancel:    func() { cancelCalls++ },
		done:      make(chan struct{}),
	}
	agent.hubbleStreams["req-dup"] = existing

	// The duplicate guard fires before HUBBLE_RELAY_ADDR is read or any stream
	// is dialed, so neither the env var nor the dialer stub needs to be set.
	agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
		RequestID: "req-dup",
		Target:    "hubble",
		Query:     "environment=development&namespace=team-1",
	})

	chunks := decodeChunks(t, mock.getWrittenMessages())
	require.Len(t, chunks, 1, "rejected duplicate should emit exactly one close chunk")
	assert.Equal(t, "req-dup", chunks[0].RequestID)
	assert.True(t, chunks[0].IsClose)
	assert.Contains(t, string(chunks[0].Data), "duplicate hubble stream requestID")

	assert.Same(t, existing, agent.hubbleStreams["req-dup"],
		"duplicate init must not evict the original session")
	assert.Zero(t, cancelCalls,
		"the original session's cancel must not fire from the rejected duplicate")
}

// On a non-EOF stream error the handler still exits cleanly and emits the
// terminal close chunk; the seam's closer must still fire (asserted by
// stubOpenHubbleFlowStream's cleanup).
func TestAgent_HandleHubbleStreamInit_StreamErrorAfterFlows(t *testing.T) {
	t.Setenv("HUBBLE_RELAY_ADDR", "hubble-relay.test.svc:4245")

	stream := &fakeFlowStream{
		responses: []*observer.GetFlowsResponse{inScopeFlow(t)},
		endErr:    errors.New("relay closed unexpectedly"),
	}
	stubOpenHubbleFlowStream(t, stream, nil)

	agent, mock := newHubbleTestAgent(t)

	done := make(chan struct{})
	go func() {
		agent.handleHubbleStreamInit(context.Background(), &messaging.HTTPTunnelStreamInit{
			RequestID: "req-6",
			Target:    "hubble",
			Query:     "environment=development&namespace=team-1",
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit after stream returned error")
	}

	chunks := decodeChunks(t, mock.getWrittenMessages())
	// sentinel + 1 flow + final close.
	require.Len(t, chunks, 3)
	assert.True(t, chunks[2].IsClose)
	assert.Empty(t, agent.hubbleStreams)
}
