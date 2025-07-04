// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
)

// RequestLoggingMiddleware creates a structured logging middleware
func RequestLoggingMiddleware(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// Add request ID if available
			requestID := c.Request().Header.Get("X-Request-ID")
			if requestID != "" {
				c.Set("request_id", requestID)
			}

			// Execute the handler
			err := next(c)

			// Log the request
			duration := time.Since(start)

			logFields := []interface{}{
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"query", c.Request().URL.RawQuery,
				"status", c.Response().Status,
				"duration", duration,
				"user_agent", c.Request().UserAgent(),
				"remote_ip", c.RealIP(),
			}

			if requestID != "" {
				logFields = append(logFields, "request_id", requestID)
			}

			// Add user information if available
			if claims := GetUserClaims(c); claims != nil {
				logFields = append(logFields,
					"user_id", claims.UserID,
					"org_id", claims.OrganizationID,
				)
			}

			if err != nil {
				logFields = append(logFields, "error", err)
				logger.Error("Request failed", logFields...)
			} else {
				// Log level based on status code
				if c.Response().Status >= 500 {
					logger.Error("Request completed with server error", logFields...)
				} else if c.Response().Status >= 400 {
					logger.Warn("Request completed with client error", logFields...)
				} else {
					logger.Info("Request completed", logFields...)
				}
			}

			return err
		}
	}
}
