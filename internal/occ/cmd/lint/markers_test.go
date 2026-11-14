// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lint_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIOutputMarkers(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "valid.yaml", validComponent)
	for _, tc := range []struct {
		name, noColor, marker string
		flag                  bool
	}{
		{name: "default_glyph", marker: "✓ No issues found"},
		{name: "environment_plain", noColor: "1", marker: "OK No issues found"},
		{name: "flag_plain", flag: true, marker: "OK No issues found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"linter", "vali", "-p", "1"}
			if tc.flag {
				args = append(args, "--no-color")
			}
			args = append(args, filepath.Join(dir, "valid.yaml"))
			cmd := exec.Command(binPath, args...)
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "NO_COLOR=") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "NO_COLOR="+tc.noColor)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("CLI: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), tc.marker) {
				t.Fatalf("expected %q in %s", tc.marker, output)
			}
		})
	}
}
