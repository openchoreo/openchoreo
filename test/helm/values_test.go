// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// helmValuesPath is the control-plane chart's values file. It holds the
// operator's own resource tree rules and the built-in opt-out switch; the
// built-in rules themselves are deliberately not here, because values are
// overridable by --set and -f.
const helmValuesPath = controlPlaneChart + "/values.yaml"

// builtInRulesPath is the chart file carrying the built-in traversal rules. No
// Helm value reaches it, so changing the built-ins means forking the chart. It
// is generated from ResourceTreeDefaults() by hack/resource-tree-builtin-rules
// via `make helm-generate.openchoreo-control-plane`; the test below proves the
// generated spelling round-trips through the real koanf loader.
const builtInRulesPath = controlPlaneChart + "/files/resource-tree-builtin-rules.yaml"

// helmResourceTreeValues is the values-side shape of the resource tree section.
// The rules themselves stay untyped so they reach the config loader with the
// spelling an operator actually writes, rather than one Go has already
// normalized.
type helmResourceTreeValues struct {
	OpenchoreoAPI struct {
		Config struct {
			ResourceTree struct {
				DisableBuiltInRules bool  `yaml:"disableBuiltInRules"`
				Rules               []any `yaml:"rules"`
			} `yaml:"resourceTree"`
		} `yaml:"config"`
	} `yaml:"openchoreoApi"`
}

// loadChartBuiltInRules reads the chart's built-in rule file as an untyped list,
// for the same reason the values rules stay untyped: the config loader has to
// see the spelling the file actually carries.
func loadChartBuiltInRules(t *testing.T) []any {
	t.Helper()

	raw, err := os.ReadFile(builtInRulesPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", builtInRulesPath, err)
	}

	var rules []any
	if err := yaml.Unmarshal(raw, &rules); err != nil {
		t.Fatalf("failed to parse %s: %v", builtInRulesPath, err)
	}
	return rules
}

// loadHelmResourceTreeValues reads the chart's resource tree values.
func loadHelmResourceTreeValues(t *testing.T) helmResourceTreeValues {
	t.Helper()

	raw, err := os.ReadFile(helmValuesPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", helmValuesPath, err)
	}

	var values helmResourceTreeValues
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("failed to parse %s: %v", helmValuesPath, err)
	}
	return values
}

// loadRulesAsConfig feeds rules through the same loader the API server boots
// with, so the values-side spelling is checked against the real koanf tags
// rather than against a second set of tags maintained here.
func loadRulesAsConfig(t *testing.T, rules []any) (*coreconfig.Loader, config.Config) {
	t.Helper()

	section, err := yaml.Marshal(map[string]any{
		"resource_tree": map[string]any{"rules": rules},
	})
	if err != nil {
		t.Fatalf("failed to serialize the chart rules: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, section, 0o600); err != nil {
		t.Fatalf("failed to write the test config: %v", err)
	}

	// The env prefix is deliberately not the production one, so a stray OC_API__*
	// in the environment cannot make drifted values look correct.
	loader := coreconfig.NewLoader("OC_API_HELMVALUES_TEST")
	if err := loader.LoadWithDefaults(nil, path); err != nil {
		t.Fatalf("failed to load the chart rules: %v", err)
	}

	var cfg config.Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("failed to unmarshal the chart rules: %v", err)
	}
	return loader, cfg
}

// TestChartBuiltInRules_MirrorResourceTreeDefaults loads the generated chart
// file with no Go defaults underneath, so what it compares is the file as the
// API server would actually read it. That proves the json-tag spellings the
// generator marshals with agree with the koanf tags the server loads with — a
// byte-level drift check cannot catch a json-tag rename, because the file
// would simply be regenerated to match the wrong spelling. It also catches the
// file going missing entirely. A config file replaces the built-in rules rather
// than adding to them, so a chart that ships an incomplete list silently drops
// rules from every Helm install.
func TestChartBuiltInRules_MirrorResourceTreeDefaults(t *testing.T) {
	_, cfg := loadRulesAsConfig(t, loadChartBuiltInRules(t))

	if diff := cmp.Diff(config.ResourceTreeDefaults(), cfg.ResourceTree); diff != "" {
		t.Errorf("%s must load back as config.ResourceTreeDefaults() exactly; regenerate it with "+
			"`make helm-generate.openchoreo-control-plane` (-want +got):\n%s",
			builtInRulesPath, diff)
	}
}

// TestHelmValues_ResourceTreeRulesAreValid runs the chart's shipped rules
// through both startup validators. The unknown-key check is what makes the
// values-side snake_case spelling safe: a camelCase slip such as `metadataOnly`
// unmarshals to a zero value that still matches the defaults, so only this
// catches it.
func TestHelmValues_ResourceTreeRulesAreValid(t *testing.T) {
	values := loadHelmResourceTreeValues(t)

	// The ConfigMap concatenates the chart's built-in rules with the operator's
	// own, so validate what an install would actually get.
	rules := append(loadChartBuiltInRules(t), values.OpenchoreoAPI.Config.ResourceTree.Rules...)
	loader, cfg := loadRulesAsConfig(t, rules)

	// Both validators return no errors for an empty rule list, so a broken key
	// path here would otherwise read as a pass rather than as nothing validated.
	if len(cfg.ResourceTree.Rules) == 0 {
		t.Fatalf("%s must ship at least one resource tree rule; found none at openchoreoApi.config.resourceTree",
			helmValuesPath)
	}

	// ValidateWithRaw is the entry point a booting server calls, so going through
	// it rather than the two validators directly keeps this test on the same
	// sequence startup runs. The loader carries only the resource_tree section,
	// so every other section comes from the defaults a server would start with
	// rather than from zero values that would fail unrelated checks.
	validated := config.Defaults()
	validated.ResourceTree = cfg.ResourceTree
	if err := validated.ValidateWithRaw(loader); err != nil {
		t.Errorf("%s must ship valid resource tree rules with no unknown keys, got:\n%v", helmValuesPath, err)
	}
}

// The pins above read values.yaml directly, which leaves the ConfigMap template
// between those values and the file the API server mounts entirely uncovered.
// The checks below render controlPlaneChart itself to cover it.
//
// helmGuardOverrides only clear the chart's placeholder-domain and required-value
// guards, which abort rendering before any template is produced. They say nothing
// about the resource tree.
var helmGuardOverrides = []string{
	"--set", "backstage.enabled=false",
	"--set", "openchoreoApi.http.hostnames[0]=api.example.com",
	"--set", "gateway.tls.hostname=example.com",
	"--set", "security.oidc.issuer=https://idp.example.com",
}

// renderAPIConfigYAML renders the API server ConfigMap and returns the
// config.yaml it carries, as text. extraArgs are appended to the helm
// invocation. The test is skipped when helm is not installed, so it adds
// coverage where the binary is available without making the package untestable
// where it is not.
func renderAPIConfigYAML(t *testing.T, extraArgs ...string) string {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the rendered ConfigMap check")
	}

	args := append([]string{
		"template", "openchoreo", controlPlaneChart,
		"--namespace", "openchoreo-control-plane",
		"--show-only", "templates/openchoreo-api/configmap.yaml",
	}, helmGuardOverrides...)
	args = append(args, extraArgs...)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), helm, args...)
	cmd.Stderr = &stderr
	rendered, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to render %s: %v\n%s", controlPlaneChart, err, stderr.String())
	}

	var configMap struct {
		Data struct {
			ConfigYAML string `yaml:"config.yaml"`
		} `yaml:"data"`
	}
	if err := yaml.Unmarshal(rendered, &configMap); err != nil {
		t.Fatalf("failed to parse the rendered ConfigMap: %v\n%s", err, rendered)
	}
	if configMap.Data.ConfigYAML == "" {
		t.Fatalf("the rendered ConfigMap carries no config.yaml:\n%s", rendered)
	}
	return configMap.Data.ConfigYAML
}

// renderAPIConfigMapRules renders the API server ConfigMap and returns the
// resource tree section of the config.yaml it carries, loaded through the same
// loader the server boots with.
func renderAPIConfigMapRules(t *testing.T, extraArgs ...string) config.ResourceTreeConfig {
	t.Helper()

	configYAML := renderAPIConfigYAML(t, extraArgs...)

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("failed to write the rendered config: %v", err)
	}

	// No Go defaults underneath, so a resource_tree section that stopped being
	// rendered shows up as an empty rule list rather than as the defaults.
	loader := coreconfig.NewLoader("OC_API_CONFIGMAP_TEST")
	if err := loader.LoadWithDefaults(nil, path); err != nil {
		t.Fatalf("failed to load the rendered config: %v", err)
	}

	var cfg config.Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("failed to unmarshal the rendered config: %v", err)
	}
	return cfg.ResourceTree
}

// TestHelmConfigMap_ShipsBuiltInRules renders the file every Helm install
// actually mounts. Dropping the chart file's rules from the template's concat
// would ship operator rules only, silently deleting all three built-in chains
// from every install, and the values-side pins would stay green throughout.
func TestHelmConfigMap_ShipsBuiltInRules(t *testing.T) {
	resourceTree := renderAPIConfigMapRules(t)

	if diff := cmp.Diff(config.ResourceTreeDefaults(), resourceTree); diff != "" {
		t.Errorf("the rendered ConfigMap must carry the built-in rules unchanged (-want +got):\n%s", diff)
	}
}

// TestHelmConfigMap_AppendsOperatorRules pins the other half of the concat: an
// operator's own rules have to reach the config in addition to the built-ins,
// after them, rather than in place of them.
func TestHelmConfigMap_AppendsOperatorRules(t *testing.T) {
	const operatorRuleJSON = `openchoreoApi.config.resourceTree.rules=[{` +
		`"root":{"group":"gateway.networking.k8s.io","version":"v1","kind":"Gateway","resource":"gateways"},` +
		`"children":[{"kind":{"version":"v1","kind":"Service","resource":"services"}}]}]`

	resourceTree := renderAPIConfigMapRules(t, "--set-json", operatorRuleJSON)

	want := append(append([]config.ResourceTreeRule{}, config.ResourceTreeDefaults().Rules...), config.ResourceTreeRule{
		Root:     config.KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Resource: "gateways"},
		Children: []config.ChildRule{{Kind: config.KindRef{Version: "v1", Kind: "Service", Resource: "services"}}},
	})

	if diff := cmp.Diff(want, resourceTree.Rules); diff != "" {
		t.Errorf("the rendered ConfigMap must append operator rules after the built-ins (-want +got):\n%s", diff)
	}
}

// TestHelmValues_ResourceTreeRulesDefaultEmpty pins `rules` as the operator's
// own list. Shipping an entry here would both surprise operators and risk a
// duplicate root against the chart's built-in rules, which fails startup
// validation. An empty default does not mean an empty effective rule set: the
// built-ins are concatenated ahead of this list.
func TestHelmValues_ResourceTreeRulesDefaultEmpty(t *testing.T) {
	values := loadHelmResourceTreeValues(t)

	if rules := values.OpenchoreoAPI.Config.ResourceTree.Rules; len(rules) > 0 {
		t.Errorf("%s must default resourceTree.rules to an empty list, got %d rule(s): %v",
			helmValuesPath, len(rules), rules)
	}
}

// TestHelmConfigMap_DisableBuiltInRulesDropsThem pins the opt-out. With the
// switch on, the rendered config must carry the operator's rules and nothing
// else — a built-in leaking through would mean an operator who asked for a
// different rule set silently got the defaults as well, and a duplicate root
// would then fail startup.
func TestHelmConfigMap_DisableBuiltInRulesDropsThem(t *testing.T) {
	const operatorRuleJSON = `openchoreoApi.config.resourceTree.rules=[{` +
		`"root":{"group":"gateway.networking.k8s.io","version":"v1","kind":"Gateway","resource":"gateways"},` +
		`"children":[{"kind":{"version":"v1","kind":"Service","resource":"services"}}]}]`

	resourceTree := renderAPIConfigMapRules(t,
		"--set", "openchoreoApi.config.resourceTree.disableBuiltInRules=true",
		"--set-json", operatorRuleJSON)

	want := []config.ResourceTreeRule{{
		Root:     config.KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Resource: "gateways"},
		Children: []config.ChildRule{{Kind: config.KindRef{Version: "v1", Kind: "Service", Resource: "services"}}},
	}}

	if diff := cmp.Diff(want, resourceTree.Rules); diff != "" {
		t.Errorf("disableBuiltInRules must leave only the operator's rules (-want +got):\n%s", diff)
	}
}

// TestHelmConfigMap_DisableBuiltInRulesWithNoRulesRendersEmpty pins the one
// remaining way to reach zero rules. It is deliberate — an operator asked for it
// with two explicit settings — and it must render as an empty list rather than
// as null.
//
// The assertion is on the rendered YAML node, not on the loaded config, because
// the loader cannot tell the two apart: `rules: null` and `rules: []` both
// unmarshal to zero rules, so a rule-count check stays green even if the
// template's trailing `default list` — the whole reason concat's nil slice does
// not reach the file — is deleted. Only the node kind pins that.
func TestHelmConfigMap_DisableBuiltInRulesWithNoRulesRendersEmpty(t *testing.T) {
	configYAML := renderAPIConfigYAML(t,
		"--set", "openchoreoApi.config.resourceTree.disableBuiltInRules=true")

	var rendered struct {
		ResourceTree struct {
			Rules yaml.Node `yaml:"rules"`
		} `yaml:"resource_tree"`
	}
	if err := yaml.Unmarshal([]byte(configYAML), &rendered); err != nil {
		t.Fatalf("failed to parse the rendered config: %v\n%s", err, configYAML)
	}

	rules := rendered.ResourceTree.Rules
	if rules.Kind != yaml.SequenceNode {
		t.Fatalf("resource_tree.rules must render as a YAML list, got kind %d tag %q value %q in:\n%s",
			rules.Kind, rules.Tag, rules.Value, configYAML)
	}
	if len(rules.Content) != 0 {
		t.Errorf("disableBuiltInRules with no operator rules must render an empty rule list, got %d entries in:\n%s",
			len(rules.Content), configYAML)
	}
}

// TestHelmValues_DisableBuiltInRulesDefaultsOff pins the shipped default. A
// chart that shipped this on would disable child discovery for every install
// that did not override it.
func TestHelmValues_DisableBuiltInRulesDefaultsOff(t *testing.T) {
	values := loadHelmResourceTreeValues(t)

	if values.OpenchoreoAPI.Config.ResourceTree.DisableBuiltInRules {
		t.Errorf("%s must default resourceTree.disableBuiltInRules to false", helmValuesPath)
	}
}

// TestHelmValues_NoBuiltInRulesKey pins the removal itself. The template no
// longer reads a builtInRules value, so re-adding one to values.yaml would be
// silently ignored — the operator would edit rules that never reach the server.
// The schema's additionalProperties: false rejects it for operators; this
// catches it for us.
func TestHelmValues_NoBuiltInRulesKey(t *testing.T) {
	raw, err := os.ReadFile(helmValuesPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", helmValuesPath, err)
	}

	var values struct {
		OpenchoreoAPI struct {
			Config struct {
				ResourceTree map[string]any `yaml:"resourceTree"`
			} `yaml:"config"`
		} `yaml:"openchoreoApi"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("failed to parse %s: %v", helmValuesPath, err)
	}

	if _, found := values.OpenchoreoAPI.Config.ResourceTree["builtInRules"]; found {
		t.Errorf("%s must not carry resourceTree.builtInRules; "+
			"the built-in rules live in %s and the template ignores any values key",
			helmValuesPath, builtInRulesPath)
	}
}

// renderDoctoredChart copies the control-plane chart to a temp directory, lets
// mutate damage the copy's built-in rules file, and returns helm's combined
// output and error. The real chart's file is correct by construction, so the
// guards under test can only be exercised against a deliberately broken copy.
func renderDoctoredChart(t *testing.T, mutate func(t *testing.T, rulesPath string)) (string, error) {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the doctored chart check")
	}

	chartCopy := filepath.Join(t.TempDir(), "chart")
	if err := os.CopyFS(chartCopy, os.DirFS(controlPlaneChart)); err != nil {
		t.Fatalf("failed to copy the chart: %v", err)
	}

	mutate(t, filepath.Join(chartCopy, "files", "resource-tree-builtin-rules.yaml"))

	args := append([]string{
		"template", "openchoreo", chartCopy,
		"--namespace", "openchoreo-control-plane",
		"--show-only", "templates/openchoreo-api/configmap.yaml",
	}, helmGuardOverrides...)

	out, err := exec.CommandContext(t.Context(), helm, args...).CombinedOutput()
	return string(out), err
}

// TestHelmConfigMap_RejectsBrokenBuiltInRulesFile covers every way the chart
// file can be wrong. fromYamlArray fails none of them on its own: missing,
// empty, `null` and `[]` all yield an empty list, and a map or scalar yields a
// one-element list holding the parse error as a string, which is non-empty and
// so survives an emptiness check while rendering garbage as a rule. Each case
// must abort the render rather than ship a config with no children.
// Each case asserts the message of the guard that owns it, not merely that the
// render failed. All three guards name the file, so a filename-only assertion
// cannot tell them apart and every case would still pass with two of the three
// guards deleted — the chained-guard defect that let six tests on this branch
// pass for the wrong reason.
func TestHelmConfigMap_RejectsBrokenBuiltInRulesFile(t *testing.T) {
	tests := []struct {
		name        string
		wantMessage string
		mutate      func(t *testing.T, rulesPath string)
	}{
		{"missing", "missing or empty", func(t *testing.T, rulesPath string) {
			if err := os.Remove(rulesPath); err != nil {
				t.Fatalf("failed to remove the rules file: %v", err)
			}
		}},
		{"empty", "missing or empty", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "")
		}},
		{"null", "parsed to an empty rule list", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "null\n")
		}},
		{"empty list", "parsed to an empty rule list", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "[]\n")
		}},
		{"comments only", "parsed to an empty rule list", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "# every rule commented out\n")
		}},
		{"map instead of list", "is not a list of rules", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "root:\n  kind: Deployment\n")
		}},
		{"scalar instead of list", "is not a list of rules", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "just-a-string\n")
		}},
		{"mixed map and scalar list", "is not a list of rules", func(t *testing.T, rulesPath string) {
			writeRulesFile(t, rulesPath, "- root:\n    version: v1\n    kind: Pod\n    resource: pods\n  children: []\n- broken\n")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := renderDoctoredChart(t, tt.mutate)
			if err == nil {
				t.Fatalf("rendering must fail when the built-in rules file is %s, got a successful render:\n%s",
					tt.name, out)
			}
			if !strings.Contains(out, "resource-tree-builtin-rules.yaml") {
				t.Errorf("the failure must name the offending file, got:\n%s", out)
			}
			if !strings.Contains(out, tt.wantMessage) {
				t.Errorf("a %s file must be rejected by the guard that owns it, whose message contains %q; got:\n%s",
					tt.name, tt.wantMessage, out)
			}
		})
	}
}

// writeRulesFile replaces the doctored chart's built-in rules file.
func writeRulesFile(t *testing.T, rulesPath, content string) {
	t.Helper()

	if err := os.WriteFile(rulesPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write the rules file: %v", err)
	}
}

// TestHelmConfigMap_DisabledBuiltInsIgnoreBrokenFile pins the guards to the
// branch that reads the file. With disableBuiltInRules on, the chart file is
// never read, so a broken one must not block an install that asked for its own
// rule set.
func TestHelmConfigMap_DisabledBuiltInsIgnoreBrokenFile(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the doctored chart check")
	}

	chartCopy := filepath.Join(t.TempDir(), "chart")
	if err := os.CopyFS(chartCopy, os.DirFS(controlPlaneChart)); err != nil {
		t.Fatalf("failed to copy the chart: %v", err)
	}
	if err := os.Remove(filepath.Join(chartCopy, "files", "resource-tree-builtin-rules.yaml")); err != nil {
		t.Fatalf("failed to remove the rules file: %v", err)
	}

	args := append([]string{
		"template", "openchoreo", chartCopy,
		"--namespace", "openchoreo-control-plane",
		"--show-only", "templates/openchoreo-api/configmap.yaml",
		"--set", "openchoreoApi.config.resourceTree.disableBuiltInRules=true",
	}, helmGuardOverrides...)

	if out, err := exec.CommandContext(t.Context(), helm, args...).CombinedOutput(); err != nil {
		t.Fatalf("rendering with disableBuiltInRules must not read the chart file, got:\n%s", out)
	}
}
