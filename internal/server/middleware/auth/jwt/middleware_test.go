// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-for-hmac-signing"

// createTestToken creates a JWT token for testing
func createTestToken(claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(testSecret))
	return tokenString
}

func TestMiddleware_Success(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": "test-issuer",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:     []byte(testSecret),
		ValidateIssuer: "test-issuer",
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify claims are in context
		ctxClaims, ok := GetClaims(r)
		if !ok {
			t.Error("Claims not found in context")
			return
		}

		if ctxClaims["sub"] != "user123" {
			t.Errorf("Expected sub claim to be 'user123', got %v", ctxClaims["sub"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddleware_CaseInsensitiveBearerScheme(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey: []byte(testSecret),
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	testCases := []struct {
		name   string
		scheme string
	}{
		{"lowercase bearer", "bearer"},
		{"uppercase BEARER", "BEARER"},
		{"mixed case Bearer", "Bearer"},
		{"mixed case BeArEr", "BeArEr"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tc.scheme+" "+token)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200 for scheme '%s', got %d", tc.scheme, w.Code)
			}
		})
	}
}

func TestMiddleware_MissingToken(t *testing.T) {
	config := Config{
		SigningKey: []byte(testSecret),
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when token is missing")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	_ = json.NewDecoder(w.Body).Decode(&response)
	if response["error"] != "MISSING_TOKEN" {
		t.Errorf("Expected error code 'missing_token', got %s", response["error"])
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	config := Config{
		SigningKey: []byte(testSecret),
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	_ = json.NewDecoder(w.Body).Decode(&response)
	if response["error"] != "INVALID_TOKEN" {
		t.Errorf("Expected error code 'invalid_token', got %s", response["error"])
	}
}

func TestMiddleware_ExpiredToken(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(-time.Hour).Unix(), // Expired 1 hour ago
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey: []byte(testSecret),
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with expired token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestMiddleware_InvalidIssuer(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"iss": "wrong-issuer",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:     []byte(testSecret),
		ValidateIssuer: "expected-issuer",
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid issuer")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}

	var response map[string]string
	_ = json.NewDecoder(w.Body).Decode(&response)
	if response["error"] != "INVALID_CLAIMS" {
		t.Errorf("Expected error code 'invalid_claims', got %s", response["error"])
	}
}

func TestMiddleware_ValidAudience(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"aud": "expected-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:        []byte(testSecret),
		ValidateAudiences: []string{"expected-audience"},
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddleware_InvalidAudience(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"aud": "wrong-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:        []byte(testSecret),
		ValidateAudiences: []string{"expected-audience"},
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid audience")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestMiddleware_NoAudienceValidationWhenNotConfigured(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
		// No audience claim
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey: []byte(testSecret),
		// ValidateAudiences is not set
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d (audience validation should be skipped)", w.Code)
	}
}

func TestMiddleware_TokenFromQuery(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:  []byte(testSecret),
		TokenLookup: "query:token",
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test?token="+token, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddleware_TokenFromCookie(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:  []byte(testSecret),
		TokenLookup: "cookie:jwt",
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "jwt", Value: token})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddleware_SuccessHandler(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":  "user123",
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	token := createTestToken(claims)

	successHandlerCalled := false
	config := Config{
		SigningKey: []byte(testSecret),
		SuccessHandler: func(w http.ResponseWriter, r *http.Request, claims jwt.MapClaims) error {
			successHandlerCalled = true
			// Custom authorization logic
			if claims["role"] != "admin" {
				return fmt.Errorf("insufficient permissions")
			}
			return nil
		},
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if !successHandlerCalled {
		t.Error("Success handler was not called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddleware_SuccessHandlerRejects(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":  "user123",
		"role": "user", // Not admin
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey: []byte(testSecret),
		SuccessHandler: func(w http.ResponseWriter, r *http.Request, claims jwt.MapClaims) error {
			if claims["role"] != "admin" {
				return fmt.Errorf("insufficient permissions")
			}
			return nil
		},
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when success handler rejects")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}

	var response map[string]string
	_ = json.NewDecoder(w.Body).Decode(&response)
	if response["error"] != "AUTHORIZATION_FAILED" {
		t.Errorf("Expected error code 'authorization_failed', got %s", response["error"])
	}
}

func TestGetClaimValue(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey: []byte(testSecret),
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub, ok := GetSubject(r)
		if !ok {
			t.Error("Subject not found in context")
			return
		}

		if sub != "user123" {
			t.Errorf("Expected subject 'user123', got %s", sub)
		}

		token, ok := GetToken(r)
		if !ok || token == "" {
			t.Error("Token not found in context")
		}

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddleware_ArrayAudience(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user123",
		"aud": []interface{}{"audience1", "expected-audience", "audience3"},
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	token := createTestToken(claims)

	config := Config{
		SigningKey:        []byte(testSecret),
		ValidateAudiences: []string{"expected-audience"},
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for valid audience in array, got %d", w.Code)
	}
}

func TestMiddleware_Disabled(t *testing.T) {
	config := Config{
		Disabled: true,
		// No signing key or JWKS URL needed when disabled
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))

	// Request without any authentication
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 when middleware is disabled, got %d", w.Code)
	}

	if w.Body.String() != "success" {
		t.Errorf("Expected body 'success', got %s", w.Body.String())
	}
}

func TestMiddleware_DisabledWithInvalidToken(t *testing.T) {
	config := Config{
		Disabled: true,
	}

	middleware := Middleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request with invalid token should still pass through when disabled
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 when middleware is disabled (even with invalid token), got %d", w.Code)
	}
}
