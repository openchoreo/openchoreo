package middleware

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/openchoreo/openchoreo/internal/logger/config"
)

// AuthMiddleware provides JWT authentication middleware
type AuthMiddleware struct {
	config *config.AuthConfig
	logger *zap.Logger
}

// Claims represents the JWT claims structure
type Claims struct {
	UserID         string   `json:"user_id"`
	OrganizationID string   `json:"org_id"`
	ProjectIDs     []string `json:"project_ids"`
	ComponentIDs   []string `json:"component_ids"`
	Roles          []string `json:"roles"`
	jwt.RegisteredClaims
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(config *config.AuthConfig, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		config: config,
		logger: logger,
	}
}

// JWTAuth returns an Echo middleware function for JWT authentication
func (a *AuthMiddleware) JWTAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip authentication if disabled
			if !a.config.EnableAuth {
				return next(c)
			}

			// Extract token from Authorization header
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				a.logger.Warn("Missing Authorization header")
				return echo.NewHTTPError(http.StatusUnauthorized, "Missing authorization header")
			}

			// Check Bearer token format
			const bearerPrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearerPrefix) {
				a.logger.Warn("Invalid Authorization header format")
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid authorization header format")
			}

			tokenString := authHeader[len(bearerPrefix):]

			// Parse and validate JWT token
			token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, echo.NewHTTPError(http.StatusUnauthorized, "Invalid signing method")
				}
				return []byte(a.config.JWTSecret), nil
			})

			if err != nil {
				a.logger.Warn("Failed to parse JWT token", zap.Error(err))
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
			}

			if !token.Valid {
				a.logger.Warn("Invalid JWT token")
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
			}

			// Extract claims
			claims, ok := token.Claims.(*Claims)
			if !ok {
				a.logger.Warn("Failed to extract claims from token")
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token claims")
			}

			// Validate required role
			if a.config.RequiredRole != "" && !a.hasRole(claims.Roles, a.config.RequiredRole) {
				a.logger.Warn("Insufficient permissions",
					zap.String("user_id", claims.UserID),
					zap.Strings("user_roles", claims.Roles),
					zap.String("required_role", a.config.RequiredRole))
				return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
			}

			// Store claims in context for use in handlers
			c.Set("user_claims", claims)

			a.logger.Debug("Authentication successful",
				zap.String("user_id", claims.UserID),
				zap.String("org_id", claims.OrganizationID))

			return next(c)
		}
	}
}

// AuthorizeComponent checks if the user has access to a specific component
func (a *AuthMiddleware) AuthorizeComponent() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip if auth is disabled
			if !a.config.EnableAuth {
				return next(c)
			}

			claims := a.getClaimsFromContext(c)
			if claims == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "No authentication claims found")
			}

			componentID := c.Param("componentId")
			if componentID != "" {
				if !a.hasAccess(claims.ComponentIDs, componentID) {
					a.logger.Warn("Component access denied",
						zap.String("user_id", claims.UserID),
						zap.String("component_id", componentID),
						zap.Strings("allowed_components", claims.ComponentIDs))
					return echo.NewHTTPError(http.StatusForbidden, "Access denied to component")
				}
			}

			return next(c)
		}
	}
}

// AuthorizeProject checks if the user has access to a specific project
func (a *AuthMiddleware) AuthorizeProject() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip if auth is disabled
			if !a.config.EnableAuth {
				return next(c)
			}

			claims := a.getClaimsFromContext(c)
			if claims == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "No authentication claims found")
			}

			projectID := c.Param("projectId")
			if projectID != "" {
				if !a.hasAccess(claims.ProjectIDs, projectID) {
					a.logger.Warn("Project access denied",
						zap.String("user_id", claims.UserID),
						zap.String("project_id", projectID),
						zap.Strings("allowed_projects", claims.ProjectIDs))
					return echo.NewHTTPError(http.StatusForbidden, "Access denied to project")
				}
			}

			return next(c)
		}
	}
}

// AuthorizeOrganization checks if the user has access to a specific organization
func (a *AuthMiddleware) AuthorizeOrganization() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip if auth is disabled
			if !a.config.EnableAuth {
				return next(c)
			}

			claims := a.getClaimsFromContext(c)
			if claims == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "No authentication claims found")
			}

			orgID := c.Param("orgId")
			if orgID != "" {
				if claims.OrganizationID != orgID {
					a.logger.Warn("Organization access denied",
						zap.String("user_id", claims.UserID),
						zap.String("requested_org", orgID),
						zap.String("user_org", claims.OrganizationID))
					return echo.NewHTTPError(http.StatusForbidden, "Access denied to organization")
				}
			}

			return next(c)
		}
	}
}

// getClaimsFromContext extracts claims from Echo context
func (a *AuthMiddleware) getClaimsFromContext(c echo.Context) *Claims {
	claims, ok := c.Get("user_claims").(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// hasRole checks if the user has a specific role
func (a *AuthMiddleware) hasRole(userRoles []string, requiredRole string) bool {
	for _, role := range userRoles {
		if role == requiredRole || role == "admin" { // Admin role has access to everything
			return true
		}
	}
	return false
}

// hasAccess checks if the user has access to a specific resource ID
func (a *AuthMiddleware) hasAccess(allowedIDs []string, requestedID string) bool {
	for _, id := range allowedIDs {
		if id == requestedID {
			return true
		}
	}
	return false
}

// GetUserClaims extracts user claims from the request context
func GetUserClaims(c echo.Context) *Claims {
	claims, ok := c.Get("user_claims").(*Claims)
	if !ok {
		return nil
	}
	return claims
}
