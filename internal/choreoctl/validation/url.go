// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateURL validates a generic URL
func ValidateURL(val interface{}) error {
	str, ok := val.(string)
	if !ok {
		return fmt.Errorf("invalid type for URL")
	}

	if str == "" {
		return fmt.Errorf("empty field for URL")
	}

	if _, err := url.Parse(str); err != nil {
		return fmt.Errorf("invalid URL format")
	}

	return nil
}

// ValidateGitHubURL validates that a URL is a proper GitHub repository URL
func ValidateGitHubURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("git repository URL is required")
	}

	if !strings.HasPrefix(urlStr, "https://github.com/") {
		return fmt.Errorf("only GitHub URLs are supported (format: https://github.com/owner/repo)")
	}

	// Validate repository path format (owner/repo)
	parts := strings.TrimPrefix(urlStr, "https://github.com/")
	if !strings.Contains(parts, "/") || strings.Count(parts, "/") > 1 {
		return fmt.Errorf("invalid GitHub repository format (expected: https://github.com/owner/repo)")
	}

	return nil
}
