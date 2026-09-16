// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package observerclient builds authenticated observer API clients for occ.
package observerclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/occ/auth"
)

// New builds an observer client that sends token and refreshes it once expired.
func New(observerURL, token string) (*obsgen.ClientWithResponses, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	return obsgen.NewClientWithResponses(
		observerURL,
		obsgen.WithHTTPClient(httpClient),
		obsgen.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			currentToken := token
			if currentToken != "" && auth.IsTokenExpired(currentToken) {
				newToken, err := auth.RefreshToken()
				if err != nil {
					return fmt.Errorf("failed to refresh token: %w", err)
				}
				currentToken = newToken
				token = newToken
			}
			if currentToken != "" {
				req.Header.Set("Authorization", "Bearer "+currentToken)
			}
			return nil
		}),
	)
}
