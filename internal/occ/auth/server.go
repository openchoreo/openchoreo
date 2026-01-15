// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	// CallbackPath is the path for the OAuth callback
	CallbackPath = "/auth-callback"
	// CallbackPort is the fixed port for the OAuth callback server
	CallbackPort = 55152
	// AuthTimeout is the timeout for waiting for authentication callback
	AuthTimeout = 5 * time.Minute
)

// AuthResult holds the result of the authentication callback
type AuthResult struct {
	Code string
	Err  error
}

// ListenForAuthCode starts a local HTTP server on the fixed callback port and waits for the auth callback
func ListenForAuthCode(expectedState string, timeout time.Duration) (string, error) {
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(CallbackPort),
		ReadHeaderTimeout: 10 * time.Second,
	}
	authCodeChan := make(chan AuthResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		// Check for OAuth error response
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			authCodeChan <- AuthResult{Err: fmt.Errorf("authentication failed: %s", errMsg)}
			writeErrorHTML(w, errMsg)
			return
		}

		// Validate state parameter to prevent CSRF
		receivedState := r.URL.Query().Get("state")
		if receivedState != expectedState {
			errMsg := "state mismatch - possible CSRF attack"
			authCodeChan <- AuthResult{Err: fmt.Errorf(errMsg)}
			writeErrorHTML(w, errMsg)
			return
		}

		// Get authorization code
		authCode := r.URL.Query().Get("code")
		if authCode == "" {
			errMsg := "no authorization code received"
			authCodeChan <- AuthResult{Err: fmt.Errorf(errMsg)}
			writeErrorHTML(w, errMsg)
			return
		}

		authCodeChan <- AuthResult{Code: authCode}
		writeSuccessHTML(w)
	})
	server.Handler = mux

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			authCodeChan <- AuthResult{Err: fmt.Errorf("server error: %w", err)}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	defer func() {
		_ = server.Shutdown(ctx)
	}()

	select {
	case result := <-authCodeChan:
		return result.Code, result.Err
	case <-time.After(timeout):
		return "", errors.New("authentication timed out after 5 minutes")
	}
}
