// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lint_test

// TestCLIContractGolden records the linter's observable behavior as a golden
// file: exit code, stdout and stderr for every documented invocation.
//
// The fixture is inherited verbatim from the toolkit, where the linter shipped
// as its own binary. The point of inheriting it rather than writing a new one is
// that this test is the evidence that moving the linter into occ changed nothing
// a user can see: the same invocations, run against the same fork, must produce
// the same bytes. The fork's golden is the toolkit's minus four scenarios - the
// ones that only existed because the linter used to be the whole `occ` binary -
// and every other scenario is byte-for-byte identical.
//
// Everything a rewrite is expected to change is normalized away: help/usage
// blocks, temp paths, durations, version numbers, and worker counts (which are
// GOMAXPROCS and therefore machine-dependent). Everything else is pinned exactly.
//
//	go test ./internal/occ/cmd/lint/ -update
//
// regenerates cli_contract.golden. Review the diff before committing it: a
// changed exit code or message is a behavior change, not a test update.

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/tools/lint/templateconfig"
)

var updateGolden = flag.Bool("update", false, "rewrite the CLI contract golden file")

// modulePath is the fork's occ entry point. The linter has no binary of its own;
// it is reached as `occ lint`, so the contract is exercised through this one.
const modulePath = "github.com/openchoreo/openchoreo/cmd/occ"

// versionPackage is linked so that `occ lint version` reports a real semver.
// internal/version defaults to "not-set", which the version normalizer would not
// mask and which would therefore make the golden depend on how the test binary
// happened to be built.
const versionPackage = "github.com/openchoreo/openchoreo/internal/version.version"

var binPath string

// TestMain builds the occ binary once, so the scenarios exercise the real
// process: entry point, flag parsing, exit codes, and the stdout/stderr split.
// Building here rather than expecting a prebuilt binary keeps the test
// self-contained; the linter's per-process state (color, selected template
// version) also cannot leak between scenarios this way.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "occ-lint-test-")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "occ")

	build := exec.Command("go", "build", "-o", binPath, "-ldflags", "-X "+versionPackage+"=1.2.3", modulePath)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(dir)
		panic(fmt.Sprintf("building occ binary: %v", err))
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type scenario struct {
	name string
	args []string
}

var (
	durationRe = regexp.MustCompile(`\b\d+(\.\d+)?(ns|µs|ms|s)\b`)
	versionRe  = regexp.MustCompile(`\b\d+\.\d+\.\d+\b`)
	// parallelRe masks the -v worker count, which is GOMAXPROCS and therefore
	// machine-dependent.
	parallelRe = regexp.MustCompile(`(parallel: )\d+`)
	// workersRe masks the "N workers" summary, for the same reason.
	workersRe = regexp.MustCompile(`\(\d+ workers\)`)
)

// usageSentinels mark the start of a trailing usage/help block, which is
// replaced wholesale rather than line by line.
var usageSentinels = []string{
	"\nUsage:",
	"\nusage:",
	"\nUsage of vali:",
	"\nFlags:",
	"\n  occ linter",
	"\n  occ lint",
	"\nOpenChoreo linter",
}

func normalize(s, fixtureDir, configPath string) string {
	s = strings.ReplaceAll(s, fixtureDir, "<FIXTURE>")
	if configPath != "" {
		s = strings.ReplaceAll(s, configPath, "<CONFIG>")
	}
	s = durationRe.ReplaceAllString(s, "<DUR>")
	s = versionRe.ReplaceAllString(s, "<VER>")
	s = parallelRe.ReplaceAllString(s, "${1}<N>")
	s = workersRe.ReplaceAllString(s, "(<N> workers)")
	return trimUsage(s)
}

// trimUsage replaces the trailing usage/help block with a sentinel. Help text is
// expected to change as commands move between binaries; everything before it is
// not.
func trimUsage(s string) string {
	for _, sentinel := range usageSentinels {
		if i := strings.Index(s, sentinel); i >= 0 {
			return s[:i] + "\n<HELP PRINTED>"
		}
	}
	return s
}

// scenarios are the toolkit's 45 invocations minus the four that only exist
// because the linter used to be the whole `occ` binary:
//
//   - usage-renamed-config    (`occ config`) is a real occ command here
//   - usage-renamed-version   (`occ version`) is a real occ command here
//   - usage-renamed-templates (`occ templates`) was a linter-ism
//   - usage-no-args           (`occ` alone) prints occ's own help now
//
// The arguments are kept in the toolkit's `linter` spelling rather than the
// canonical `lint` so the recorded command lines stay byte-identical to the
// inherited fixture. `occ lint` is the same command reached through the alias,
// and lintAliasMatchesLinter covers that spelling directly.
var scenarios = []scenario{
	{name: "vali-clean-file", args: []string{"linter", "vali", "{dir}/valid.yaml"}},
	{name: "vali-invalid-file", args: []string{"linter", "vali", "{dir}/invalid.yaml"}},
	{name: "vali-json-invalid", args: []string{"linter", "vali", "-obj", "{dir}/invalid.yaml"}},
	{name: "vali-json-clean", args: []string{"linter", "vali", "-obj", "{dir}/valid.yaml"}},
	{name: "vali-batch-brief", args: []string{"linter", "vali", "-b", "{dir}"}},
	{name: "vali-detect-folder", args: []string{"linter", "vali", "-detect", "-p", "1", "{dir}"}},
	{name: "vali-fix", args: []string{"linter", "vali", "-fix", "{dir}/typo.yaml"}},
	{name: "vali-fix-dry-run", args: []string{"linter", "vali", "-fix", "-dry-run", "{dir}/typo.yaml"}},
	{name: "vali-quiet", args: []string{"linter", "vali", "-detect", "-q", "-p", "1", "{dir}"}},
	{name: "vali-verbose", args: []string{"linter", "vali", "-detect", "-v", "-p", "1", "{dir}"}},
	{name: "vali-no-color", args: []string{"linter", "vali", "-detect", "--no-color", "-p", "1", "{dir}"}},
	{name: "vali-explicit-template", args: []string{"linter", "vali", "-template-version", "v1.2.0", "{dir}/invalid.yaml"}},
	{name: "vali-double-dash-obj", args: []string{"linter", "vali", "--obj", "{dir}/invalid.yaml"}},
	{name: "vali-double-dash-dry-run", args: []string{"linter", "vali", "--dry-run", "{dir}/typo.yaml"}},
	{name: "schema-list", args: []string{"linter", "schema", "list"}},
	{name: "schema-select", args: []string{"linter", "schema", "select", "v1.2.0"}},
	{name: "version-subcommand", args: []string{"linter", "version"}},
	{name: "version-flag", args: []string{"linter", "--version"}},
	{name: "lint-alias-matches-linter", args: []string{"lint", "vali", "{dir}/invalid.yaml"}},

	// The dispatch surface. There is no default subcommand: "occ linter -b dir"
	// is an error, and this has to keep being true now that the linter is a
	// subtree rather than a root.
	{name: "linter-missing-subcommand", args: []string{"linter"}},
	{name: "linter-unknown-subcommand-flag", args: []string{"linter", "-b", "{dir}"}},
	{name: "linter-unknown-subcommand-path", args: []string{"linter", "{dir}/valid.yaml"}},
	{name: "linter-not-a-command", args: []string{"linter", "config"}},
	{name: "linter-renamed-templates", args: []string{"linter", "templates"}},
	{name: "linter-renamed-version", args: []string{"linter", "version", "x"}},
	{name: "linter-help", args: []string{"linter", "help"}},
	{name: "linter-help-flag", args: []string{"linter", "--help"}},

	// Arity and flag-spelling edges.
	{name: "vali-extra-positional-ignored", args: []string{"linter", "vali", "{dir}/invalid.yaml", "ignored"}},
	{name: "vali-long-flag-batch", args: []string{"linter", "vali", "--batch", "-p", "1", "{dir}"}},
	{name: "vali-long-flag-parallel", args: []string{"linter", "vali", "--parallel", "1", "{dir}/invalid.yaml"}},
	{name: "vali-missing-flag-value", args: []string{"linter", "vali", "-p"}},
	{name: "vali-invalid-flag-value", args: []string{"linter", "vali", "-p", "abc", "{dir}/valid.yaml"}},
	{name: "vali-unknown-long-flag", args: []string{"linter", "vali", "--nope", "{dir}/valid.yaml"}},
	{name: "vali-help", args: []string{"linter", "vali", "-h"}},
	{name: "schema-missing-subcommand", args: []string{"linter", "schema"}},
	{name: "schema-unknown-subcommand", args: []string{"linter", "schema", "nope"}},
	{name: "schema-select-no-version", args: []string{"linter", "schema", "select"}},

	// Usage errors, all of which must exit 2.
	{name: "usage-dry-run-without-fix", args: []string{"linter", "vali", "-dry-run", "{dir}/typo.yaml"}},
	{name: "usage-missing-path", args: []string{"linter", "vali"}},
	{name: "usage-unknown-flag", args: []string{"linter", "vali", "-nope", "{dir}/valid.yaml"}},
	{name: "usage-unknown-command", args: []string{"linter", "nope"}},
}

func TestCLIContractGolden(t *testing.T) {
	var sb strings.Builder
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(t.TempDir(), "config.json")
			t.Setenv(templateconfig.EnvConfigPath, configPath)

			writeFixture(t, dir, "valid.yaml", validComponent)
			writeFixture(t, dir, "invalid.yaml", invalidComponent)
			writeFixture(t, dir, "typo.yaml", typoComponent)

			args := make([]string, len(sc.args))
			for i, a := range sc.args {
				args[i] = strings.ReplaceAll(a, "{dir}", dir)
			}

			code, stdout, stderr := runCLI(t, args...)

			fmt.Fprintf(&sb, "### %s\n$ occ %s\nexit: %d\n--- stdout ---\n%s\n--- stderr ---\n%s\n",
				sc.name, normalize(strings.Join(args, " "), dir, configPath), code,
				normalize(stdout, dir, configPath), normalize(stderr, dir, configPath))
		})
	}

	goldenPath := "cli_contract.golden"
	if *updateGolden {
		if err := os.WriteFile(goldenPath, []byte(sb.String()), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote %s (%d bytes, %d scenarios)", goldenPath, sb.Len(), len(scenarios))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	got := sb.String()
	if got != string(want) {
		t.Fatalf("CLI contract changed.\n%s", firstDiff(string(want), got))
	}
	t.Logf("%d scenarios match %s", len(scenarios), goldenPath)
}

// runCLI runs the built occ binary and returns exit code, stdout and stderr.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	// Golden contracts use plain markers regardless of the invoking environment.
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "HOME="+authenticatedHome(t))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("running %s %v: %v", binPath, args, err)
		}
		code = ee.ExitCode()
	}
	return code, out.String(), errBuf.String()
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// firstDiff reports the first differing line plus a little context, so a failure
// points at the behavior that moved rather than dumping the whole file.
func firstDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("first difference at line %d:\n  golden: %q\n  actual: %q", i+1, w, g)
		}
	}
	return "outputs are equal line by line but differ in length"
}

func authenticatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte("test-only"))
	if err != nil {
		t.Fatal(err)
	}
	testutil.WriteOCConfig(t, home, &config.StoredConfig{
		CurrentContext: "test", ControlPlanes: []config.ControlPlane{{Name: "test", URL: "http://127.0.0.1:1"}}, Credentials: []config.Credential{{Name: "test", Token: token}},
		Contexts: []config.Context{{Name: "test", ControlPlane: "test", Credentials: "test"}},
	})
	return home
}
