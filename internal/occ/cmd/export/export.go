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

// Run prepares the complete export before transactionally synchronizing managed
// files. Layouts determine ownership; no inventory is written into the output.
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
	// Do not prune files using a view that is now known to be permission-limited.
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
	}, nil
}

// A filtered list cannot distinguish an absent object from an inaccessible one.
// Full-cluster export therefore requires provable, unconditional access to every
// supported kind before fetching and again before synchronizing the destination.
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
	// View action names follow the API's lower-case kind naming convention.
	// Reuse the supported layouts so newly exported kinds cannot bypass the gate.
	for kind := range resourceLayouts {
		action := strings.ToLower(kind) + ":view"
		if !hasUnrestrictedAccess(*profile.Capabilities, action) {
			return errExportPermissionDenied
		}
	}
	return nil
}

// Keep generated-client access local to export instead of extending the shared
// resource client interface. The existing accessor preserves authentication and
// token refresh configured by client.NewClient.
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
	// No namespace or project selectors: scoped grants must not appear global.
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
	// Normal profiles expand action wildcards. The disabled authorizer can return
	// a '*' capability; its grants must never override a concrete action denial.
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

// Resolve existing ancestors so output paths are consistent even when the
// destination does not exist yet or is reached through a symlink.
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
