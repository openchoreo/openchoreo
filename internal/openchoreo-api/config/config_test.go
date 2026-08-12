// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// loadConfig loads the built-in defaults, then configPath on top, the same way
// openchoreo-api does at startup. The env prefix is deliberately not the
// production one so a stray OC_API__* in the environment cannot change results.
func loadConfig(t *testing.T, configPath string) (*coreconfig.Loader, Config) {
	t.Helper()

	loader := coreconfig.NewLoader("OC_API_CONFIG_TEST")
	if err := loader.LoadWithDefaults(Defaults(), configPath); err != nil {
		t.Fatalf("failed to load %s: %v", configPath, err)
	}

	var cfg Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("failed to unmarshal %s: %v", configPath, err)
	}
	return loader, cfg
}

func TestConfigValidate_ResourceTreeRules(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr string // substring of the error text; "" = valid
	}{
		{"custom ownerRef rule", "resource_tree_ownerref.yaml", ""},
		{"labelSelector rule", "resource_tree_label_selector.yaml", ""},
		{
			"objectRef is not implemented yet",
			"resource_tree_object_ref.yaml",
			`resource_tree.rules[0].children[0].matcher: matcher "objectRef" is not supported by this binary`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cfg := loadConfig(t, filepath.Join("testdata", tt.file))

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the config to be valid, got:\n%v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q, got none", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected an error containing %q, got:\n%v", tt.wantErr, err)
			}
		})
	}
}

// TestConfigValidate_ResourceTreeRulesReplaceDefaults pins koanf's merge
// behavior for the rules list: a config file replaces the built-in rules rather
// than adding to them. The chart file files/resource-tree-builtin-rules.yaml is
// the only other copy of these rules.
func TestConfigValidate_ResourceTreeRulesReplaceDefaults(t *testing.T) {
	_, cfg := loadConfig(t, filepath.Join("testdata", "resource_tree_ownerref.yaml"))

	if len(cfg.ResourceTree.Rules) != 1 {
		t.Fatalf("expected the file's single rule to replace the %d built-in rules, got %d",
			len(ResourceTreeDefaults().Rules), len(cfg.ResourceTree.Rules))
	}
	if got := cfg.ResourceTree.Rules[0].Root.Kind; got != "Service" {
		t.Errorf("expected the file's rule, got root kind %q", got)
	}
}

// TestConfigValidate_ResourceTreeDefaultsApply covers the section being absent:
// the built-in rules must still reach cfg.ResourceTree. Round-tripping the
// defaults through koanf turns a leaf's nil Children into an empty slice, which
// every reader treats the same, so empty and nil compare equal here.
func TestConfigValidate_ResourceTreeDefaultsApply(t *testing.T) {
	_, cfg := loadConfig(t, "")

	if diff := cmp.Diff(ResourceTreeDefaults(), cfg.ResourceTree, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("expected the built-in rules with no config file (-want +got):\n%s", diff)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected the built-in rules to be valid, got:\n%v", err)
	}
}

// TestConfigValidate_ResourceTreeUnknownKeyNeedsRawCheck is why startup runs both
// validators: unmarshaling drops a misspelled key, so Validate() alone sees a
// perfectly valid config and only the raw section reveals the typo.
func TestConfigValidate_ResourceTreeUnknownKeyNeedsRawCheck(t *testing.T) {
	loader, cfg := loadConfig(t, filepath.Join("testdata", "resource_tree_unknown_key.yaml"))

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected the unmarshaled config to look valid, got:\n%v", err)
	}

	errs := cfg.ResourceTree.validateRawKeys(loader.RawAt("resource_tree"))
	if len(errs) == 0 {
		t.Fatal("expected the raw check to reject the misspelled metadata_onlyy key")
	}
	if got := errs.Error(); !strings.Contains(got, "metadata_onlyy") {
		t.Errorf("expected the error to name the unknown key, got:\n%s", got)
	}
}

// writeConfig writes a config file to a temporary directory and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write the test config: %v", err)
	}
	return path
}

// TestValidateWithRaw_MergesUnknownKeyAndRuleErrors pins that startup reports both
// classes of resource_tree defect in one pass. Unknown keys are only visible in
// the raw config and invalid rules only in the unmarshaled one, so reporting
// them separately would let the first failure hide the second.
func TestValidateWithRaw_MergesUnknownKeyAndRuleErrors(t *testing.T) {
	path := writeConfig(t, `
resource_tree:
  rules:
    - root:
        group: apps
        version: v1
        kind: Deployment
        resource: deployments
      children:
        - kind:
            version: v1
            kind: Pod
            resource: pods
          matcher: objectRef
          metadata_onlyy: true
`)
	loader, cfg := loadConfig(t, path)

	err := cfg.ValidateWithRaw(loader)
	if err == nil {
		t.Fatal("expected the invalid configuration to be rejected")
	}

	// main() reports each field and message separately, so the merged error has
	// to stay a ValidationErrors.
	var validationErrs coreconfig.ValidationErrors
	if !errors.As(err, &validationErrs) {
		t.Fatalf("expected coreconfig.ValidationErrors, got %T: %v", err, err)
	}

	// Each defect is named by its dotted path, the form main() logs.
	got := validationErrs.Error()
	for _, want := range []string{
		`resource_tree.rules[0].children[0]: unknown key "metadata_onlyy"`,
		`resource_tree.rules[0].children[0].matcher: matcher "objectRef" is not supported`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected an error containing %q, got:\n%s", want, got)
		}
	}
}
