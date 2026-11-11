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
