// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpCallTimeout = 30 * time.Second

var httpClientWithTimeout = &http.Client{Timeout: mcpCallTimeout}

// MCPClientConfig holds configuration for creating an MCP client session.
type MCPClientConfig struct {
	Endpoint               string
	Token                  string
	Toolsets                []string
	FilterByAuthz          *bool
	IncludeDeprecatedTools *bool
}

// bearerTransport injects an Authorization header into every outgoing request.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.base.RoundTrip(req)
}

// NewMCPSession creates an MCP client session connected to the given endpoint.
// The caller must close the returned session when done.
func NewMCPSession(ctx context.Context, cfg MCPClientConfig) (*mcp.ClientSession, error) {
	endpoint := cfg.Endpoint
	if len(cfg.Toolsets) > 0 || cfg.FilterByAuthz != nil || cfg.IncludeDeprecatedTools != nil {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid MCP endpoint URL: %w", err)
		}
		q := u.Query()
		if len(cfg.Toolsets) > 0 {
			q.Set("toolsets", strings.Join(cfg.Toolsets, ","))
		}
		if cfg.FilterByAuthz != nil {
			q.Set("filterByAuthz", fmt.Sprintf("%t", *cfg.FilterByAuthz))
		}
		if cfg.IncludeDeprecatedTools != nil {
			q.Set("includeDeprecatedTools", fmt.Sprintf("%t", *cfg.IncludeDeprecatedTools))
		}
		u.RawQuery = q.Encode()
		endpoint = u.String()
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Transport: &bearerTransport{
				token: cfg.Token,
				base:  http.DefaultTransport,
			},
		},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, mcpCallTimeout)
		defer cancel()
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "1.0.0",
	}, nil)

	return client.Connect(ctx, transport, nil)
}

// ListMCPToolNames lists tools and returns just the tool names.
func ListMCPToolNames(session *mcp.ClientSession) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpCallTimeout)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, t := range result.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

// CallMCPTool calls a named tool with the given arguments and returns the result.
// Returns an error if the tool call fails or the tool itself returns an error.
func CallMCPTool(session *mcp.ClientSession, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpCallTimeout)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return result, fmt.Errorf("tool %s returned error: %s", toolName, extractTextContent(result))
	}
	return result, nil
}

// CallMCPToolJSON calls a named tool and unmarshals the first text content into dest.
func CallMCPToolJSON(session *mcp.ClientSession, toolName string, args map[string]any, dest any) error {
	result, err := CallMCPTool(session, toolName, args)
	if err != nil {
		return fmt.Errorf("call %s failed: %w", toolName, err)
	}
	text := extractTextContent(result)
	if text == "" {
		return fmt.Errorf("tool %s returned no text content", toolName)
	}
	if dest != nil {
		if err := json.Unmarshal([]byte(text), dest); err != nil {
			return fmt.Errorf("failed to unmarshal %s result: %w\nraw: %s", toolName, err, text)
		}
	}
	return nil
}

func extractTextContent(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// MCPRawPOST sends a raw HTTP POST to the MCP endpoint without any token.
// Useful for testing unauthenticated access (401 response).
func MCPRawPOST(endpoint string) (int, http.Header, error) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"e2e-test","version":"1.0.0"},"capabilities":{}}}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := httpClientWithTimeout.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, nil
}

// FetchOAuth2Token requests an access token from the Thunder IdP using
// client_credentials grant with HTTP Basic auth (client_secret_basic).
func FetchOAuth2Token(tokenURL, clientID, clientSecret string) (string, error) {
	data := url.Values{
		"grant_type": {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := httpClientWithTimeout.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("unmarshal token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response: %s", string(body))
	}
	return tokenResp.AccessToken, nil
}
