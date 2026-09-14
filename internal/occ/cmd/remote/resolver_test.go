// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// writeResolverCredential stores one credential whose access token is unusable, so any
// resolve has to refresh before it can send one.
func writeResolverCredential(t *testing.T, controlPlaneURL string) {
	t.Helper()
	if err := config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: controlPlaneURL}},
		Credentials: []config.Credential{{
			Name:         "cred",
			Token:        "stale-token",
			RefreshToken: "rt",
			ClientID:     "cli-id",
			AuthMethod:   "authorization_code",
		}},
		Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// futureJWT returns a token IsTokenExpired accepts, so callToken uses it as-is. The
// subject keeps tokens minted in the same second distinguishable.
func futureJWT(t *testing.T, subject string) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix(), "sub": subject}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// A refresh usually fails for transient reasons, so it must not reach the renewer
// looking like the permanent refusal a 401 would be.
func TestResolveReportsRefreshFailureAsRetryable(t *testing.T) {
	const cpURL = "http://mock-control-plane"
	testutil.SetupTestHome(t)
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/") || r.URL.Path == "/token" {
			return nil, errors.New("identity provider unreachable")
		}
		// What the control plane answers when the stale token is sent anyway.
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader("token expired")),
			Header:     http.Header{},
		}, nil
	}))
	writeResolverCredential(t, cpURL)

	h := &httpResolver{
		baseURL:         cpURL,
		controlPlaneURL: cpURL,
		credentialName:  "cred",
		client:          &http.Client{},
	}
	_, err := h.Resolve(t.Context(), remoteconnect.ResolveRequest{Project: "p", Component: "c"})
	if err == nil {
		t.Fatal("a resolve that could not obtain a token should fail")
	}
	var statusErr *resolveStatusError
	if errors.As(err, &statusErr) {
		t.Errorf("a refresh failure became a status error the renewer treats as permanent: %v", err)
	}
}

// The credential is pinned at construction, so switching context elsewhere cannot
// repoint a running session at another control plane's token.
func TestCallTokenUsesThePinnedCredential(t *testing.T) {
	const cpURL = "http://mock-control-plane"
	testutil.SetupTestHome(t)
	writeResolverCredential(t, cpURL)

	cfg, err := config.LoadStoredConfig()
	if err != nil {
		t.Fatal(err)
	}
	// Both credentials are usable as-is, so whichever one is read is the one returned.
	pinnedToken, otherToken := futureJWT(t, "pinned"), futureJWT(t, "other")
	cred, err := cfg.CredentialByName("cred")
	if err != nil {
		t.Fatal(err)
	}
	cred.Token = pinnedToken
	// A second context becomes current after the resolver pinned the first.
	cfg.Credentials = append(cfg.Credentials, config.Credential{Name: "other", Token: otherToken})
	cfg.Contexts = append(cfg.Contexts, config.Context{Name: "other", ControlPlane: "cp", Credentials: "other"})
	cfg.CurrentContext = "other"
	if err := config.SaveStoredConfig(cfg); err != nil {
		t.Fatal(err)
	}

	h := &httpResolver{baseURL: cpURL, controlPlaneURL: cpURL, credentialName: "cred", client: &http.Client{}}
	token, err := h.callToken()
	if err != nil {
		t.Fatalf("callToken: %v", err)
	}
	if token == otherToken {
		t.Error("callToken followed the current context instead of the pinned credential")
	}
	if token != pinnedToken {
		t.Errorf("callToken returned %q, want the pinned credential's token", token)
	}
}

// A credential can hold redeemable refresh material with no access token yet, and that
// is still a session the resolver can authenticate.
func TestCallTokenRefreshesAnEmptyAccessToken(t *testing.T) {
	const cpURL = "http://mock-control-plane"
	issued := futureJWT(t, "refreshed")
	testutil.SetupTestHome(t)
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body any
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			body = map[string]any{
				"authorization_servers":       []string{cpURL},
				"openchoreo_clients":          []map[string]any{{"name": "cli", "client_id": "cli-id", "scopes": []string{"openid"}}},
				"openchoreo_security_enabled": true,
			}
		case "/.well-known/openid-configuration":
			body = map[string]any{"authorization_endpoint": cpURL + "/authorize", "token_endpoint": cpURL + "/token"}
		case "/token":
			body = map[string]any{"access_token": issued, "token_type": "Bearer", "expires_in": 3600}
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
		}
		b, merr := json.Marshal(body)
		if merr != nil {
			return nil, merr
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(b))),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}))

	if err := config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: cpURL}},
		Credentials: []config.Credential{{
			Name: "cred", Token: "", RefreshToken: "rt", ClientID: "cli-id", AuthMethod: "authorization_code",
		}},
		Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	h := &httpResolver{baseURL: cpURL, controlPlaneURL: cpURL, credentialName: "cred", client: &http.Client{}}
	token, err := h.callToken()
	if err != nil {
		t.Fatalf("callToken: %v", err)
	}
	if token != issued {
		t.Errorf("callToken returned %q, want the refreshed token", token)
	}
}

// A credential with nothing to redeem yields what it has rather than failing the resolve.
func TestCallTokenLeavesUnredeemableCredentialAlone(t *testing.T) {
	const cpURL = "http://mock-control-plane"
	testutil.SetupTestHome(t)
	if err := config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: cpURL}},
		Credentials:    []config.Credential{{Name: "cred"}},
		Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	h := &httpResolver{baseURL: cpURL, controlPlaneURL: cpURL, credentialName: "cred", client: &http.Client{}}
	token, err := h.callToken()
	if err != nil {
		t.Fatalf("callToken: %v", err)
	}
	if token != "" {
		t.Errorf("callToken returned %q, want an empty bearer", token)
	}
}
