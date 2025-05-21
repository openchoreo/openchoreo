/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package kubernetes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxNameLength = 253
	hashLength    = 8   // Length of the hash suffix
	separator     = "-" // Separator between name parts

	// Max length limits for Kubernetes resource names

	MaxLabelNameLength     = 63
	MaxResourceNameLength  = maxNameLength
	MaxCronJobNameLength   = 52
	MaxJobNameLength       = 63
	MaxServiceNameLength   = 63
	MaxNamespaceNameLength = 63
	MaxContainerNameLength = 63
	MaxVolumeNameLength    = 63
)

// GenerateK8sName generates a Kubernetes-compliant name within the length limit,
// ensuring uniqueness by appending a hash of the full concatenated names.
// See https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
// NOTE: Changes to this function will impact the generated names of all resources that can cause resource
// recreation and stale resources in the k8s cluster.
func GenerateK8sName(names ...string) string {
	return GenerateK8sNameWithLengthLimit(maxNameLength, names...)
}

// GenerateK8sNameWithLengthLimit generates a Kubernetes-compliant name within the given length limit.
// This is useful when the name must be within a specific length limit, that is different from the default limit.
// Example: CronJob names must be within 52 characters.
func GenerateK8sNameWithLengthLimit(limit int, names ...string) string {
	// Clean and sanitize each name part
	cleanedNames := make([]string, 0, len(names))
	for _, name := range names {
		cleanedName := sanitizeName(name)
		cleanedNames = append(cleanedNames, cleanedName)
	}

	// Generate a hash from the full original concatenated names for uniqueness
	fullName := strings.Join(names, separator)
	hashBytes := sha256.Sum256([]byte(fullName))
	hashString := hex.EncodeToString(hashBytes[:])[:hashLength]

	// Calculate the maximum allowed length for the base name
	// Subtract length for separators and hash
	numberOfNames := len(cleanedNames)
	numberOfSeparatorsInBaseName := numberOfNames - 1
	totalSeparatorLength := len(separator) * numberOfSeparatorsInBaseName
	// Separator before the hash
	totalSeparatorLength += len(separator)

	maxBaseNameLength := limit - hashLength - totalSeparatorLength

	// Calculate maximum length for each name part
	maxPartLength := maxBaseNameLength / numberOfNames
	extraChars := maxBaseNameLength % numberOfNames

	// Truncate each name part to its maximum allowed length
	truncatedNames := make([]string, numberOfNames)
	for i, name := range cleanedNames {
		allocatedLength := maxPartLength
		if i < extraChars {
			// Distribute remaining characters among the first few names
			allocatedLength++
		}
		if len(name) > allocatedLength {
			truncatedNames[i] = name[:allocatedLength]
		} else {
			truncatedNames[i] = name
		}
	}
	// Concatenate the truncated names with the separator
	baseName := strings.Join(truncatedNames, separator)

	// Combine base name and hash
	finalName := fmt.Sprintf("%s%s%s", baseName, separator, hashString)

	// Ensure the final name complies with DNS subdomain conventions
	finalName = ensureDNSSubdomainCompliance(finalName)

	return finalName
}

// sanitizeName removes invalid characters and converts to lowercase
func sanitizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Remove invalid characters
	var sanitized []rune
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '.' {
			sanitized = append(sanitized, r)
		} else {
			// Replace invalid characters with '-'
			sanitized = append(sanitized, '-')
		}
	}

	// Trim any leading or trailing non-alphanumeric characters
	sanitizedName := strings.Trim(string(sanitized), "-.")

	return sanitizedName
}

// ensureDNSSubdomainCompliance ensures the name starts and ends with an alphanumeric character
func ensureDNSSubdomainCompliance(name string) string {
	// Trim invalid start characters
	name = strings.TrimLeftFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	// Trim invalid end characters
	name = strings.TrimRightFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	return name
}
