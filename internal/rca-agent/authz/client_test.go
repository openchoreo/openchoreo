// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/rca-agent/config"
)

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(&config.AuthzConfig{
		ServiceURL: serverURL,
		Timeout:    30,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	return client
}

func testEvalRequest() *authzcore.EvaluateRequest {
	return &authzcore.EvaluateRequest{
		SubjectContext: &authzcore.SubjectContext{Type: "user"},
		Action:         "rcareport:view",
		Resource: authzcore.Resource{
			Type: "project",
			ID:   "proj-1",
		},
	}
}

func TestNewClient_MissingURL(t *testing.T) {
	t.Parallel()
	_, err := NewClient(&config.AuthzConfig{
		Timeout: 30,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err)
}

func TestNewClient_InvalidTimeout(t *testing.T) {
	t.Parallel()
	_, err := NewClient(&config.AuthzConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    0,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err)
}

func TestEvaluate_Allowed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, evaluatesEndpoint, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]authzcore.Decision{
			{Decision: true, Context: &authzcore.DecisionContext{}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	decision, err := client.Evaluate(context.Background(), testEvalRequest())

	require.NoError(t, err)
	assert.True(t, decision.Decision)
}

func TestEvaluate_Denied(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]authzcore.Decision{
			{Decision: false, Context: &authzcore.DecisionContext{Reason: "no access"}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	decision, err := client.Evaluate(context.Background(), testEvalRequest())

	require.NoError(t, err)
	assert.False(t, decision.Decision)
}

func TestEvaluate_NilRequest(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Evaluate(context.Background(), nil)
	require.Error(t, err)
}

func TestEvaluate_ServerReturns401(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Evaluate(context.Background(), testEvalRequest())
	require.ErrorIs(t, err, ErrAuthzUnauthorized)
}

func TestEvaluate_ServerReturns403(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Evaluate(context.Background(), testEvalRequest())
	require.ErrorIs(t, err, ErrAuthzForbidden)
}

func TestEvaluate_ServerReturns500(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Evaluate(context.Background(), testEvalRequest())
	require.ErrorIs(t, err, ErrAuthzServiceUnavailable)
}

func TestEvaluate_ServerReturnsInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Evaluate(context.Background(), testEvalRequest())
	require.ErrorIs(t, err, ErrAuthzInvalidResponse)
}

func TestEvaluate_ServerReturnsWrongDecisionCount(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]authzcore.Decision{
			{Decision: true},
			{Decision: false},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.Evaluate(context.Background(), testEvalRequest())
	require.ErrorIs(t, err, ErrAuthzInvalidResponse)
}

func TestEvaluate_ServerUnreachable(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://127.0.0.1:1")
	_, err := client.Evaluate(context.Background(), testEvalRequest())
	require.ErrorIs(t, err, ErrAuthzServiceUnavailable)
}

func TestBatchEvaluate_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]authzcore.Decision{
			{Decision: true},
			{Decision: false},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	resp, err := client.BatchEvaluate(context.Background(), &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{*testEvalRequest(), *testEvalRequest()},
	})

	require.NoError(t, err)
	require.Len(t, resp.Decisions, 2)
	assert.True(t, resp.Decisions[0].Decision)
	assert.False(t, resp.Decisions[1].Decision)
}

func TestBatchEvaluate_NilRequest(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.BatchEvaluate(context.Background(), nil)
	require.Error(t, err)
}

func TestBatchEvaluate_CountMismatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]authzcore.Decision{
			{Decision: true},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.BatchEvaluate(context.Background(), &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{*testEvalRequest(), *testEvalRequest()},
	})
	require.ErrorIs(t, err, ErrAuthzInvalidResponse)
}
