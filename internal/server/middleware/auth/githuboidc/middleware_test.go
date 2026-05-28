// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package githuboidc_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/githuboidc"
)

// testProvider runs an in-process OIDC provider: it serves a JWKS document
// derived from an RSA keypair and signs arbitrary claim sets with that key,
// which is sufficient for go-oidc to discover and verify the resulting
// tokens.
type testProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
	issuer string
}

func newTestProvider(t *testing.T) *testProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	mux := http.NewServeMux()
	tp := &testProvider{key: key, kid: "test-key-1"}
	tp.server = httptest.NewServer(mux)
	tp.issuer = tp.server.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                tp.issuer,
			"jwks_uri":                              tp.issuer + "/jwks",
			"response_types_supported":              []string{"id_token"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": tp.kid,
				"n":   b64uint(key.PublicKey.N.Bytes()),
				"e":   b64uint(intToBytes(key.PublicKey.E)),
			}},
		})
	})
	t.Cleanup(tp.server.Close)
	return tp
}

func (tp *testProvider) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": tp.kid,
	})
	payloadJSON, _ := json.Marshal(claims)
	h := base64.RawURLEncoding.EncodeToString(headerJSON)
	p := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := h + "." + p
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, tp.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func b64uint(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func intToBytes(n int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n)) //nolint:gosec // n is the RSA public exponent (always positive, <= 65537).
	for i, v := range b {
		if v != 0 {
			return b[i:]
		}
	}
	return []byte{0}
}

func validClaims(issuer, audience string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"iss":              issuer,
		"sub":              "repo:octo-org/octo-repo:ref:refs/heads/main",
		"aud":              audience,
		"exp":              now + 600,
		"iat":              now,
		"nbf":              now,
		"repository":       "octo-org/octo-repo",
		"repository_id":    "1296269",
		"repository_owner": "octo-org",
		"ref":              "refs/heads/main",
		"sha":              "fc1234abcd",
		"workflow":         "build-and-deploy",
		"workflow_ref":     "octo-org/octo-repo/.github/workflows/deploy.yml@refs/heads/main",
		"job_workflow_ref": "octo-org/octo-repo/.github/workflows/deploy.yml@refs/heads/main",
		"run_id":           "12345",
		"run_attempt":      "1",
		"actor":            "octocat",
		"event_name":       "push",
	}
}

const testAudience = "openchoreo-test"

func newMiddleware(t *testing.T, tp *testProvider) func(http.Handler) http.Handler {
	t.Helper()
	verifier, err := githuboidc.NewVerifier(t.Context(), githuboidc.Config{
		Issuer:   tp.issuer,
		Audience: testAudience,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return githuboidc.Middleware(verifier)
}

type capture struct {
	called bool
	subj   *auth.SubjectContext
	claims *githuboidc.Claims
}

func captureNext() (http.Handler, *capture) {
	c := &capture{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.called = true
		if s, ok := auth.GetSubjectContext(r); ok {
			c.subj = s
		}
		if cl, ok := githuboidc.ClaimsFromContext(r.Context()); ok {
			c.claims = cl
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return h, c
}

func TestMiddleware_ValidToken_SetsSubjectAndClaims(t *testing.T) {
	tp := newTestProvider(t)
	mw := newMiddleware(t, tp)
	next, captured := captureNext()
	handler := mw(next)

	token := tp.sign(t, validClaims(tp.issuer, testAudience))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		body, _ := io.ReadAll(w.Body)
		t.Fatalf("expected 204, got %d: %s", w.Code, body)
	}
	if !captured.called {
		t.Fatal("next handler was not called")
	}
	if captured.subj == nil || captured.subj.Type != githuboidc.SubjectType {
		t.Fatalf("expected SubjectContext type %q, got %+v", githuboidc.SubjectType, captured.subj)
	}
	if captured.claims == nil || captured.claims.Repository != "octo-org/octo-repo" {
		t.Fatalf("expected claims.Repository = octo-org/octo-repo, got %+v", captured.claims)
	}
}

func TestMiddleware_TokenWithoutRepositoryClaim_Rejects(t *testing.T) {
	tp := newTestProvider(t)
	mw := newMiddleware(t, tp)
	next, captured := captureNext()
	handler := mw(next)

	claims := validClaims(tp.issuer, testAudience)
	delete(claims, "repository")
	token := tp.sign(t, claims)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if captured.called {
		t.Fatal("next handler was called for token missing repository claim")
	}
}

func TestMiddleware_WrongAudience_Rejects(t *testing.T) {
	tp := newTestProvider(t)
	mw := newMiddleware(t, tp)
	next, captured := captureNext()
	handler := mw(next)

	token := tp.sign(t, validClaims(tp.issuer, "some-other-audience"))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if captured.called {
		t.Fatal("next handler was called for token with wrong audience")
	}
}

func TestMiddleware_ExpiredToken_Rejects(t *testing.T) {
	tp := newTestProvider(t)
	mw := newMiddleware(t, tp)
	next, captured := captureNext()
	handler := mw(next)

	claims := validClaims(tp.issuer, testAudience)
	claims["exp"] = time.Now().Add(-1 * time.Hour).Unix()
	token := tp.sign(t, claims)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if captured.called {
		t.Fatal("next handler was called for expired token")
	}
}

func TestMiddleware_MissingAuthHeader_Rejects(t *testing.T) {
	tp := newTestProvider(t)
	mw := newMiddleware(t, tp)
	next, captured := captureNext()
	handler := mw(next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if captured.called {
		t.Fatal("next handler was called without Authorization header")
	}
}

func TestMiddleware_DisabledVerifier_Passthrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := githuboidc.Middleware(nil)
	handler := mw(next)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Fatal("expected next to be called when middleware is disabled")
	}
}

func TestNewVerifier_RequiresAudience(t *testing.T) {
	tp := newTestProvider(t)
	_, err := githuboidc.NewVerifier(t.Context(), githuboidc.Config{Issuer: tp.issuer})
	if err == nil || !strings.Contains(err.Error(), "Audience") {
		t.Fatalf("expected audience-required error, got %v", err)
	}
}

func TestNewVerifier_BadIssuer_FailsAtConstructionTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := githuboidc.NewVerifier(t.Context(), githuboidc.Config{
		Issuer:   srv.URL,
		Audience: "openchoreo-test",
	})
	if err == nil {
		t.Fatal("expected error from bad issuer discovery, got nil")
	}
}
