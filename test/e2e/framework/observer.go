// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Observer service coordinates inside the e2e cluster. Set by `_e2e.install-op`
// in make/e2e.mk — keep these names in sync.
const (
	ObserverNamespace = "openchoreo-observability-plane"
	ObserverService   = "observer"
	ObserverPort      = 8080

	// Thunder OAuth2 client_credentials path used to mint a bearer token for
	// the observer's JWT-protected query routes. service_mcp_client carries
	// the `admin` ClusterAuthzRoleBinding (install/helm/openchoreo-control-plane/
	// values.yaml `mcp-tryout-client-binding`) so it can call logs:view /
	// metrics:view / traces:view — the canonical "view everything" service
	// account in the in-cluster IdP, and the right shape for an e2e test
	// that needs broad observability access.
	//
	// The `openchoreo-observer-resource-reader-client` looks like the obvious
	// choice from its name but only grants UID resolution permissions
	// (component:view / project:view / namespace:view / environment:view),
	// not log/metric query permissions — so the observer denies its own
	// queries on those routes.
	thunderTokenURLInCluster   = "http://thunder-service.thunder.svc.cluster.local:8090/oauth2/token"
	observerOAuthClientID      = "service_mcp_client"
	observerOAuthClientSecret  = "service_mcp_client_secret" //nolint:gosec
	observerQueryHTTPTimeoutSc = 20

	// IngestionBudget is the upper bound the observability specs poll within
	// for OpenSearch ingestion lag (logs/metrics/traces queries). Centralised
	// here so a CI tuning bump only changes one place.
	IngestionBudget = 3 * time.Minute
)

// ObserverQueryFrom resolves a Running pod that has curl available and runs
// observer queries through it. The pod must live in the cluster so it can
// reach `observer.openchoreo-observability-plane.svc:8080` and acquire a
// token from `thunder-service.thunder.svc:8090`. The Gitea pod used by the
// build/gitops suites is a natural fit (it ships curl); suites that don't
// install Gitea should call `framework.DeployCurlPod` to get one.
type ObserverQueryFrom struct {
	KubeContext string
	Namespace   string
	PodLabel    string // selector, e.g. "app=gitea"
	Container   string // container name; "" picks the first
}

// LogsQueryRequest mirrors the observer's OpenAPI request body for
// POST /api/v1/logs/query. Only the fields the e2e suites need are modeled.
// SearchScope is `any` so callers can pass either ComponentSearchScope or
// WorkflowSearchScope without us re-declaring oneOf logic.
type LogsQueryRequest struct {
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	SearchScope  any       `json:"searchScope"`
	SearchPhrase *string   `json:"searchPhrase,omitempty"`
	Limit        *int      `json:"limit,omitempty"`
	SortOrder    *string   `json:"sortOrder,omitempty"`
}

// ComponentSearchScope scopes a logs/metrics/traces query to a component.
// Namespace is the openchoreo control-plane namespace the Component lives in
// (not the data-plane namespace).
type ComponentSearchScope struct {
	Namespace   string  `json:"namespace"`
	Project     *string `json:"project,omitempty"`
	Component   *string `json:"component,omitempty"`
	Environment *string `json:"environment,omitempty"`
}

// WorkflowSearchScope scopes a logs query to a WorkflowRun's logs. Used by
// `build-logs-after-deletion` to prove logs survive the CR's deletion.
type WorkflowSearchScope struct {
	Namespace       string  `json:"namespace"`
	WorkflowRunName *string `json:"workflowRunName,omitempty"`
	TaskName        *string `json:"taskName,omitempty"`
}

// LogsQueryResponse models the parts of the observer's logs response the e2e
// suites assert on.
type LogsQueryResponse struct {
	Logs  []map[string]any `json:"logs,omitempty"`
	Total *int             `json:"total,omitempty"`
}

// MetricsQueryRequest mirrors POST /api/v1/metrics/query. Metric is
// observer-defined ("resource" / "http"); Step is a Prometheus-style duration
// like "1m".
type MetricsQueryRequest struct {
	StartTime   time.Time            `json:"startTime"`
	EndTime     time.Time            `json:"endTime"`
	Metric      string               `json:"metric"`
	SearchScope ComponentSearchScope `json:"searchScope"`
	Step        *string              `json:"step,omitempty"`
}

// TracesQueryRequest mirrors POST /api/v1alpha1/traces/query.
type TracesQueryRequest struct {
	StartTime   time.Time            `json:"startTime"`
	EndTime     time.Time            `json:"endTime"`
	SearchScope ComponentSearchScope `json:"searchScope"`
	Limit       *int                 `json:"limit,omitempty"`
	SortOrder   *string              `json:"sortOrder,omitempty"`
}

// TracesQueryResponse keeps the helper independent of the full observer types.
type TracesQueryResponse struct {
	Traces []map[string]any `json:"traces,omitempty"`
	Total  *int             `json:"total,omitempty"`
}

// MetricsQueryResponse is intentionally loose — the observer returns one of
// several shapes depending on `metric` (http vs resource), and we only need
// to inspect series counts at this layer.
type MetricsQueryResponse map[string]any

// AcquireObserverToken does a client_credentials grant against Thunder from a
// pod inside the cluster and returns the access token. Cached per (caller-
// chosen) tester pod so successive query calls in one Eventually loop don't
// each hammer the IdP. Tokens have a 1h validity in the e2e bootstrap, which
// dwarfs a Ginkgo It-block.
func AcquireObserverToken(q ObserverQueryFrom) (string, error) {
	// service_mcp_client uses token_endpoint_auth_method=client_secret_basic
	// so credentials go in the Authorization header, not the form body.
	out, err := KubectlExecByLabel(q.KubeContext, q.Namespace, q.PodLabel, q.Container,
		"curl", "-sS", "--fail-with-body",
		"-m", fmt.Sprintf("%d", observerQueryHTTPTimeoutSc),
		"-u", observerOAuthClientID+":"+observerOAuthClientSecret,
		"-H", "Content-Type: application/x-www-form-urlencoded",
		"-X", "POST",
		"--data", "grant_type=client_credentials",
		thunderTokenURLInCluster,
	)
	if err != nil {
		return "", fmt.Errorf("thunder token request failed: %w (body: %s)", err, out)
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if jerr := json.Unmarshal([]byte(out), &resp); jerr != nil {
		return "", fmt.Errorf("thunder token decode failed: %w (body: %s)", jerr, out)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("thunder token response missing access_token (error=%q desc=%q body=%s)",
			resp.Error, resp.ErrorDesc, out)
	}
	return resp.AccessToken, nil
}

// QueryLogs POSTs to /api/v1/logs/query and returns the parsed response.
// Pass a bearer token acquired via AcquireObserverToken. Errors include the
// HTTP body so flakes are diagnosable from CI logs.
func QueryLogs(q ObserverQueryFrom, token string, req LogsQueryRequest) (*LogsQueryResponse, error) {
	body, err := observerPost(q, token, "/api/v1/logs/query", req)
	if err != nil {
		return nil, err
	}
	var resp LogsQueryResponse
	if jerr := json.Unmarshal([]byte(body), &resp); jerr != nil {
		return nil, fmt.Errorf("logs query decode failed: %w (body: %s)", jerr, body)
	}
	return &resp, nil
}

// QueryMetrics POSTs to /api/v1/metrics/query.
func QueryMetrics(q ObserverQueryFrom, token string, req MetricsQueryRequest) (MetricsQueryResponse, error) {
	body, err := observerPost(q, token, "/api/v1/metrics/query", req)
	if err != nil {
		return nil, err
	}
	var resp MetricsQueryResponse
	if jerr := json.Unmarshal([]byte(body), &resp); jerr != nil {
		return nil, fmt.Errorf("metrics query decode failed: %w (body: %s)", jerr, body)
	}
	return resp, nil
}

// QueryTraces POSTs to /api/v1alpha1/traces/query.
func QueryTraces(q ObserverQueryFrom, token string, req TracesQueryRequest) (*TracesQueryResponse, error) {
	body, err := observerPost(q, token, "/api/v1alpha1/traces/query", req)
	if err != nil {
		return nil, err
	}
	var resp TracesQueryResponse
	if jerr := json.Unmarshal([]byte(body), &resp); jerr != nil {
		return nil, fmt.Errorf("traces query decode failed: %w (body: %s)", jerr, body)
	}
	return &resp, nil
}

// observerPost runs `kubectl exec <pod> -- curl ...` to POST to the observer's
// in-cluster Service. Using kubectl-exec instead of `kubectl port-forward`
// avoids forwarder lifecycle management and mirrors the pattern used by
// framework/gitea.go.
func observerPost(q ObserverQueryFrom, token, path string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s",
		ObserverService, ObserverNamespace, ObserverPort, path)
	args := []string{
		"-sS", "--fail-with-body",
		"-m", fmt.Sprintf("%d", observerQueryHTTPTimeoutSc),
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer " + token,
		"-X", "POST",
		"--data", string(body),
		url,
	}
	out, err := KubectlExecByLabel(q.KubeContext, q.Namespace, q.PodLabel, q.Container,
		append([]string{"curl"}, args...)...)
	if err != nil {
		return out, fmt.Errorf("POST %s failed: %w (body: %s)", path, err, out)
	}
	return strings.TrimSpace(out), nil
}

// StringPtr is a tiny convenience to avoid scattering `s := "x"; &s` patterns
// at the call sites of the query helpers.
func StringPtr(s string) *string { return &s }

// IntPtr mirrors StringPtr for int-valued query fields.
func IntPtr(i int) *int { return &i }
