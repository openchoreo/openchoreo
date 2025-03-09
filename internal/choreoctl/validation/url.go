/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
