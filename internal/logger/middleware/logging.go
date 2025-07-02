// Copyright (c) 2025 openchoreo
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// RequestLoggingMiddleware creates a structured logging middleware
func RequestLoggingMiddleware(logger *zap.Logger) echo.MiddlewareFunc {
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

			fields := []zap.Field{
				zap.String("method", c.Request().Method),
				zap.String("path", c.Request().URL.Path),
				zap.String("query", c.Request().URL.RawQuery),
				zap.Int("status", c.Response().Status),
				zap.Duration("duration", duration),
				zap.String("user_agent", c.Request().UserAgent()),
				zap.String("remote_ip", c.RealIP()),
			}

			if requestID != "" {
				fields = append(fields, zap.String("request_id", requestID))
			}

			// Add user information if available
			if claims := GetUserClaims(c); claims != nil {
				fields = append(fields,
					zap.String("user_id", claims.UserID),
					zap.String("org_id", claims.OrganizationID),
				)
			}

			if err != nil {
				fields = append(fields, zap.Error(err))
				logger.Error("Request failed", fields...)
			} else {
				// Log level based on status code
				if c.Response().Status >= 500 {
					logger.Error("Request completed with server error", fields...)
				} else if c.Response().Status >= 400 {
					logger.Warn("Request completed with client error", fields...)
				} else {
					logger.Info("Request completed", fields...)
				}
			}

			return err
		}
	}
}
