// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

var errExportPermissionDenied = errors.New("your role does not have enough permission to perform export")

// Run collects resources and synchronizes managed export files.
func Run(ctx context.Context, c client.Interface, params Params) (*ExportResult, error) {
	if err := params.validate(); err != nil {
		return nil, err
	}
	directory, err := canonicalOutputDir(params.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output directory: %w", err)
	}
	if err := checkExportPermissions(ctx, c); err != nil {
		return nil, err
	}
	fetched, err := Fetch(ctx, c, params.AllReleases)
	if err != nil {
		return nil, err
	}
	filtered, err := Filter(ctx, fetched.Resources)
	if err != nil {
		return nil, err
	}
	files, err := Serialize(ctx, filtered)
	if err != nil {
		return nil, err
	}
	if err := checkExportPermissions(ctx, c); err != nil {
		return nil, err
	}
	options := writeOptions{
		preservedPaths: fetched.PreservedPaths,
	}
	counts, err := writeFiles(ctx, directory, files, options)
	if err != nil {
		return nil, err
	}
	return &ExportResult{
		OutputDir: directory, Files: files, Counts: counts, ReleaseCutoff: fetched.ReleaseCutoff,
		Warnings: fetched.Warnings,
	}, nil
}

// checkExportPermissions requires unrestricted access to every supported kind.
func checkExportPermissions(ctx context.Context, c client.Interface) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("API client is required")
	}
	profile, err := getExportProfile(ctx, c)
	if err != nil {
		if errors.Is(err, errExportPermissionDenied) {
			return errExportPermissionDenied
		}
		return fmt.Errorf("cannot verify full-cluster export permissions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if profile == nil || profile.Capabilities == nil {
		return fmt.Errorf("cannot verify full-cluster export permissions: authorization capabilities are missing")
	}
	// Check the view action for every supported kind.
	for kind := range resourceLayouts {
		action := strings.ToLower(kind) + ":view"
		if !hasUnrestrictedAccess(*profile.Capabilities, action) {
			return errExportPermissionDenied
		}
	}
	return nil
}

// generatedClientProvider exposes the generated client for profile and raw list calls.
type generatedClientProvider interface {
	GetClient() *gen.ClientWithResponses
}

func getExportProfile(ctx context.Context, c client.Interface) (*gen.UserCapabilitiesResponse, error) {
	provider, ok := c.(generatedClientProvider)
	if !ok {
		return nil, fmt.Errorf("API client does not expose the generated client required to verify export permissions")
	}
	api := provider.GetClient()
	if api == nil {
		return nil, fmt.Errorf("generated API client is missing")
	}
	resp, err := api.GetSubjectProfileWithResponse(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get authorization profile: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("authorization profile response is missing")
	}
	if resp.StatusCode() == http.StatusForbidden {
		return nil, errExportPermissionDenied
	}
	if resp.JSON200 == nil {
		var problem gen.ErrorResponse
		if err := json.Unmarshal(resp.Body, &problem); err == nil && problem.Error != "" {
			return nil, fmt.Errorf("authorization profile request failed (HTTP %d): %s", resp.StatusCode(), problem.Error)
		}
		return nil, fmt.Errorf("authorization profile request failed (HTTP %d)", resp.StatusCode())
	}
	return resp.JSON200, nil
}

func hasUnrestrictedAccess(capabilities map[string]gen.ActionCapability, action string) bool {
	allowed := false
	// Check wildcard grants while preserving explicit action denials.
	for _, name := range []string{action, "*"} {
		capability := capabilities[name]
		if capability.Denied != nil && len(*capability.Denied) != 0 {
			return false
		}
		if capability.Allowed == nil {
			continue
		}
		for _, resource := range *capability.Allowed {
			if resource.Path == nil || *resource.Path != "*" {
				continue
			}
			constraints := resource.Constraints
			if constraints == nil || constraints.Expressions == nil || len(*constraints.Expressions) == 0 {
				allowed = true
			}
		}
	}
	return allowed
}

// canonicalOutputDir resolves an absolute output path through existing symlinks.
func canonicalOutputDir(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	ancestor := absolute
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			relative, err := filepath.Rel(ancestor, absolute)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		}
		if !os.IsNotExist(err) || filepath.Dir(ancestor) == ancestor {
			return "", err
		}
		ancestor = filepath.Dir(ancestor)
	}
}
