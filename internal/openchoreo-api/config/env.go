// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

// Environment variable names for OpenChoreo API configuration
const (
	// EnvServerBaseURL is the base URL for the API server (used for OAuth metadata)
	EnvServerBaseURL = "SERVER_BASE_URL"

	// EnvAuthServerBaseURL is the base URL for Asgardeo Thunder (authorization server)
	EnvAuthServerBaseURL = "AUTH_SERVER_BASE_URL"

	// EnvJWKSURL is the JWKS URL for JWT validation
	EnvJWKSURL = "JWKS_URL"

	// EnvJWTIssuer is the expected JWT issuer
	EnvJWTIssuer = "JWT_ISSUER"

	// EnvJWTAudience is the expected JWT audience (optional)
	EnvJWTAudience = "JWT_AUDIENCE"

	// EnvMCPToolsets is the comma-separated list of enabled MCP toolsets
	EnvMCPToolsets = "MCP_TOOLSETS"

	// EnvJWTDisabled is the flag to disable JWT authentication
	EnvJWTDisabled = "JWT_DISABLED"

	// EnvLogLevel is the log level for the API server (debug, info, warn, error)
	EnvLogLevel = "LOG_LEVEL"
)

// Default values for configuration
const (
	DefaultServerBaseURL  = "http://api.openchoreo.localhost"
	DefaultThunderBaseURL = "http://sts.openchoreo.localhost"
)
