// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"fmt"
	"strings"
)

const (
	// secretTypeLabel marks a SecretReference's category (e.g. git
	// credentials). The key matches the one used by the Backstage plugins
	// (openchoreo.dev/secret-type) so the CLI and UI agree on categories.
	secretTypeLabel = "openchoreo.dev/secret-type"

	// categoryGeneric is the default category, emitted as the value
	// of the secret-type label when no other category is specified.
	categoryGeneric = "generic"

	// categoryGitCredentials marks a secret that holds git credentials.
	// Workflows and CI build dialogs discover git secrets by this value.
	categoryGitCredentials = "git-credentials"
)

// knownCategories lists every category accepted by the CLI.
var knownCategories = []string{categoryGeneric, categoryGitCredentials}

// labelsFor returns the secret-type label for the given category (defaulting to generic), or an error if the category is unknown.
func labelsFor(category string) (map[string]string, error) {
	if category == "" {
		category = categoryGeneric
	}
	switch category {
	case categoryGeneric, categoryGitCredentials:
		return map[string]string{secretTypeLabel: category}, nil
	default:
		return nil, fmt.Errorf("invalid --category %q: must be one of %s", category, strings.Join(knownCategories, ", "))
	}
}
