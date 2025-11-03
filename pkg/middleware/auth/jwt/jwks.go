// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWK represents a JSON Web Key
type JWK struct {
	Kid string   `json:"kid"`
	Kty string   `json:"kty"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	Alg string   `json:"alg"`
	X5c []string `json:"x5c,omitempty"`
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// jwksCache holds the cached JWKS and related metadata
type jwksCache struct {
	mu              sync.RWMutex
	keys            map[string]*rsa.PublicKey
	lastRefresh     time.Time
	jwksURL         string
	refreshInterval time.Duration
	httpClient      *http.Client
	logger          *slog.Logger
}

// getKey retrieves a public key from the cache by key ID
func (c *jwksCache) getKey(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key, exists := c.keys[kid]
	if !exists {
		return nil, fmt.Errorf("key with kid '%s' not found in JWKS", kid)
	}
	return key, nil
}

// refresh fetches the JWKS from the URL and updates the cache
func (c *jwksCache) refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to refresh
	if time.Since(c.lastRefresh) < c.refreshInterval {
		return nil
	}

	c.logger.Debug("Refreshing JWKS", "url", c.jwksURL)

	resp, err := c.httpClient.Get(c.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	// Parse and store keys
	newKeys := make(map[string]*rsa.PublicKey)
	for _, jwk := range jwks.Keys {
		if jwk.Kty != "RSA" {
			c.logger.Debug("Skipping non-RSA key", "kid", jwk.Kid, "kty", jwk.Kty)
			continue
		}

		key, err := parseRSAPublicKeyFromJWK(&jwk)
		if err != nil {
			c.logger.Warn("Failed to parse JWK", "kid", jwk.Kid, "error", err)
			continue
		}

		newKeys[jwk.Kid] = key
	}

	if len(newKeys) == 0 {
		return errors.New("no valid RSA keys found in JWKS")
	}

	c.keys = newKeys
	c.lastRefresh = time.Now()
	c.logger.Info("JWKS refreshed successfully", "key_count", len(newKeys))

	return nil
}

// startBackgroundRefresh starts a background goroutine to periodically refresh JWKS
func (c *jwksCache) startBackgroundRefresh() {
	go func() {
		ticker := time.NewTicker(c.refreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.refresh(); err != nil {
				c.logger.Error("Failed to refresh JWKS", "error", err)
			}
		}
	}()
}

// parseRSAPublicKeyFromJWK parses an RSA public key from a JWK
func parseRSAPublicKeyFromJWK(jwk *JWK) (*rsa.PublicKey, error) {
	// Decode the modulus (n)
	nBytes, err := jwt.NewParser().DecodeSegment(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode the exponent (e)
	eBytes, err := jwt.NewParser().DecodeSegment(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert bytes to big integers
	var n big.Int
	n.SetBytes(nBytes)

	// Convert exponent bytes to int
	var e int
	for _, b := range eBytes {
		e = e*256 + int(b)
	}

	// Create RSA public key
	return &rsa.PublicKey{
		N: &n,
		E: e,
	}, nil
}
