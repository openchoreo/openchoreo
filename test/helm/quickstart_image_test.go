// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const (
	quickStartDockerfile = "../../install/quick-start/Dockerfile"
	quickStartHelpers    = "../../install/quick-start/.helpers.sh"
)

// commonValuesRef matches a reference to a shared k3d values file, however the
// helpers spell the directory (they fall back to an in-container path).
var commonValuesRef = regexp.MustCompile(`(?:k3d/common|\$thunder_dir|\$openbao_values)/?([a-z0-9.-]+\.yaml)`)

// TestQuickStartImageShipsTheValuesFilesTheInstallerUses pins the quick-start
// image's contents to what the installer inside it actually reads.
//
// The image copies the shared k3d values files one by one, so adding a values
// file to install/k3d/common and referencing it from .helpers.sh is not enough:
// the file is simply absent at runtime and helm is handed a path that does not
// exist. That is invisible outside the container and breaks the default
// ./install.sh rather than an opt-in path, which is how it escaped review once
// already.
func TestQuickStartImageShipsTheValuesFilesTheInstallerUses(t *testing.T) {
	dockerfile, err := os.ReadFile(quickStartDockerfile)
	if err != nil {
		t.Fatalf("failed to read %s: %v", quickStartDockerfile, err)
	}
	helpers, err := os.ReadFile(quickStartHelpers)
	if err != nil {
		t.Fatalf("failed to read %s: %v", quickStartHelpers, err)
	}

	wanted := map[string]bool{}
	for _, m := range commonValuesRef.FindAllStringSubmatch(string(helpers), -1) {
		wanted[m[1]] = true
	}
	if len(wanted) == 0 {
		t.Fatalf("found no k3d/common values references in %s; the matcher is probably stale",
			quickStartHelpers)
	}

	copied := string(dockerfile)
	for name := range wanted {
		if !strings.Contains(copied, "k3d/common/"+name) {
			t.Errorf("%s reads install/k3d/common/%s, but %s never copies it into the image, so "+
				"the path does not exist at runtime", quickStartHelpers, name, quickStartDockerfile)
		}
	}
}

const preloadImagesScript = "../../install/k3d/preload-images.sh"

var (
	// Flags the helpers hand to the preload script, as they appear in the
	// preload_args array ("--cp-set", "--thunder-values", ...).
	preloadFlagRef = regexp.MustCompile(`"(--[a-z][a-z0-9-]*)"`)
	// Options the preload script's own argument loop accepts, i.e. the case
	// labels "--cp-set)" and "--help|-h)".
	preloadFlagCase = regexp.MustCompile(`(?m)^\s+(--[a-z][a-z0-9-]*)[)|]`)
)

// TestQuickStartPreloadFlagsAreAccepted pins the flags the quick-start helpers
// pass to install/k3d/preload-images.sh to the ones that script parses.
//
// An unknown option makes the preload script exit non-zero, which the helpers
// downgrade to "Image preloading failed - continuing with installation". The
// install then succeeds while every image is pulled from inside the cluster, so
// the only symptom is a much slower install.
func TestQuickStartPreloadFlagsAreAccepted(t *testing.T) {
	helpers, err := os.ReadFile(quickStartHelpers)
	if err != nil {
		t.Fatalf("failed to read %s: %v", quickStartHelpers, err)
	}
	script, err := os.ReadFile(preloadImagesScript)
	if err != nil {
		t.Fatalf("failed to read %s: %v", preloadImagesScript, err)
	}

	body := functionBody(t, string(helpers), "preload_images")

	accepted := map[string]bool{}
	for _, m := range preloadFlagCase.FindAllStringSubmatch(string(script), -1) {
		accepted[m[1]] = true
	}
	if !accepted["--cluster"] {
		t.Fatalf("found no option cases in %s; the matcher is probably stale", preloadImagesScript)
	}

	passed := map[string]bool{}
	for _, m := range preloadFlagRef.FindAllStringSubmatch(body, -1) {
		passed[m[1]] = true
	}
	if !passed["--cluster"] {
		t.Fatalf("found no flags in preload_images(); the matcher is probably stale")
	}

	for flag := range passed {
		if !accepted[flag] {
			t.Errorf("preload_images() passes %s, which %s does not accept, so the whole "+
				"preload step fails with \"Unknown option\"", flag, preloadImagesScript)
		}
	}
}

// functionBody returns the body of a shell function, from its opening line to
// the closing brace in the first column.
func functionBody(t *testing.T, script, name string) string {
	t.Helper()

	_, after, found := strings.Cut(script, "\n"+name+"() {\n")
	if !found {
		t.Fatalf("no %s() definition found", name)
	}
	body, _, found := strings.Cut(after, "\n}\n")
	if !found {
		t.Fatalf("%s() has no closing brace in the first column", name)
	}
	return body
}
