// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const buildPodmanStub = `#!/bin/sh
echo "podman $*" >> "$CALLS"
case "$1" in
  info)
    echo true
    ;;
  image)
    [ "$2" = "exists" ] && exit 0
    ;;
  save)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-o" ]; then
        mkdir -p "$(dirname "$2")"
        : > "$2"
        exit 0
      fi
      shift
    done
    ;;
esac
exit 0
`

const packStub = `#!/bin/sh
echo "pack $*" >> "$CALLS"
exit 0
`

const buildJQStub = `#!/bin/sh
input=$(cat)
echo "jq $*" >> "$CALLS"
echo "jq-stdin $input" >> "$CALLS"
case "$*" in
  *"--env"*)
    printf '%s\n' "--env FOO=bar" "--env HELLO=world"
    ;;
  *"--build-arg"*)
    printf '%s\n' "--build-arg HTTP_PROXY=http://proxy"
    ;;
esac
exit 0
`

const publishPodmanStub = `#!/bin/sh
echo "podman $*" >> "$CALLS"
exit 0
`

type scriptRunResult struct {
	exitCode int
	output   string
	calls    []string
	root     string
}

func runScript(t *testing.T, script string, stubs map[string]string, setup func(root string), replacements func(root string) []string) scriptRunResult {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available; skipping behavioral test")
	}

	root := t.TempDir()
	stubDir := filepath.Join(root, "bin")
	callsFile := filepath.Join(root, "calls.log")
	require.NoError(t, os.MkdirAll(stubDir, 0o755))
	for name, content := range stubs {
		writeExec(t, filepath.Join(stubDir, name), content)
	}
	if setup != nil {
		setup(root)
	}

	if replacements != nil {
		script = strings.NewReplacer(replacements(root)...).Replace(script)
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CALLS="+callsFile,
	)
	out, err := cmd.CombinedOutput()

	res := scriptRunResult{output: string(out), root: root}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.exitCode = exitErr.ExitCode()
		} else {
			res.exitCode = -1
		}
	}
	if data, readErr := os.ReadFile(callsFile); readErr == nil {
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			if line != "" {
				res.calls = append(res.calls, line)
			}
		}
	}
	return res
}

func buildReplacements(root string) []string {
	vol := filepath.Join(root, "mnt-vol")
	return []string{
		"{{workflow.parameters.image-name}}", "example/app",
		"{{workflow.parameters.image-tag}}", "dev",
		"{{inputs.parameters.git-revision}}", "abcdef12",
		"{{workflow.parameters.docker-context}}", ".",
		"{{workflow.parameters.dockerfile-path}}", "Dockerfile",
		"{{workflow.parameters.app-path}}", "service",
		"{{inputs.parameters.build-env}}", `[{"name":"FOO","value":"bar"},{"name":"HELLO","value":"world"}]`,
		"{{workflow.parameters.build-env}}", `[{"name":"FOO","value":"bar"},{"name":"HELLO","value":"world"}]`,
		"{{inputs.parameters.build-args}}", `[{"name":"HTTP_PROXY","value":"http://proxy"}]`,
		"/mnt/vol", vol,
		"/storage/run", filepath.Join(root, "storage", "run"),
		"/storage/graph", filepath.Join(root, "storage", "graph"),
		"/etc/containers", filepath.Join(root, "containers"),
		"/run/podman", filepath.Join(root, "run", "podman"),
	}
}

func TestContainerfileBuild_Behavior(t *testing.T) {
	script := scriptForTemplate(t, "containerfile-build.yaml", "build-image")
	res := runScript(t, script, map[string]string{
		"podman": buildPodmanStub,
		"jq":     buildJQStub,
	}, func(root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol", "source"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "mnt-vol", "source", "Dockerfile"), []byte("FROM scratch\n"), 0o644))
	}, buildReplacements)

	requireScriptSuccess(t, res, "containerfile build should complete with valid Dockerfile and context")
	requireCallContains(t, res, "jq-stdin", `[{"name":"FOO","value":"bar"},{"name":"HELLO","value":"world"}]`,
		"containerfile build must pass build-env JSON into jq for --env conversion")
	requireHasCall(t, res, "podman build -t example/app:dev-abcdef12",
		"containerfile build must invoke podman build for the generated image tag")
	requireCallContains(t, res, "podman build", "--env FOO=bar",
		"containerfile build must pass build-env entries as podman --env flags")
	requireCallContains(t, res, "podman build", "--build-arg HTTP_PROXY=http://proxy",
		"containerfile build must pass build-args entries as podman --build-arg flags")
	requireCallContains(t, res, "podman save -o", filepath.Join(res.root, "mnt-vol", "app-image.tar"),
		"containerfile build must save the image tar handoff for publish-image")
}

func TestContainerfileBuild_MissingDockerfileFailsBeforeBuild(t *testing.T) {
	script := scriptForTemplate(t, "containerfile-build.yaml", "build-image")
	res := runScript(t, script, map[string]string{
		"podman": buildPodmanStub,
		"jq":     buildJQStub,
	}, func(root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol", "source"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
	}, buildReplacements)

	requireScriptExitCode(t, res, 1, "missing Dockerfile should fail before build")
	requireOutputContains(t, res, "Dockerfile not found",
		"missing Dockerfile failure should explain that the Dockerfile path is invalid")
	requireNoCall(t, res, "podman build",
		"containerfile build must not run podman build when Dockerfile validation fails")
}

func TestBuildpackBuilds_Behavior(t *testing.T) {
	for _, tc := range []struct {
		file string
	}{
		{"ballerina-buildpack-build.yaml"},
		{"gcp-buildpacks-build.yaml"},
		{"paketo-buildpacks-build.yaml"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			script := scriptForTemplate(t, tc.file, "build-image")
			res := runScript(t, script, map[string]string{
				"podman": buildPodmanStub,
				"pack":   packStub,
				"jq":     buildJQStub,
			}, func(root string) {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol", "source", "service"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
			}, buildReplacements)

			requireScriptSuccess(t, res, tc.file+" buildpack build should complete with a valid app path")
			requireHasCall(t, res, "podman system service --time=0",
				"buildpack build must start the podman service before pack build")
			requireHasCall(t, res, "podman info --format",
				"buildpack build must wait until the podman remote socket exists")
			requireCallContains(t, res, "pack build example/app:dev-abcdef12", "--docker-host inherit",
				"buildpack build must let pack use the inherited podman socket")
			requireCallContains(t, res, "pack build example/app:dev-abcdef12", "--pull-policy always",
				"buildpack build must pull builder/run images according to the template contract")
			requireCallContains(t, res, "pack build example/app:dev-abcdef12", "--builder",
				"buildpack build must pass an explicit builder image")
			requireCallContains(t, res, "pack build example/app:dev-abcdef12", "@sha256:",
				"buildpack build must use digest-pinned builder/run images")
			requireCallContains(t, res, "pack build example/app:dev-abcdef12", "--env FOO=bar",
				"buildpack build must pass build-env entries as pack --env flags")
			requireHasCall(t, res, "podman image exists example/app:dev-abcdef12",
				"buildpack build must wait until the built image exists before saving")
			requireCallContains(t, res, "podman save -o", filepath.Join(res.root, "mnt-vol", "app-image.tar"),
				"buildpack build must save the image tar handoff for publish-image")
		})
	}
}

func TestBuildpackBuild_MissingAppPathFailsBeforePack(t *testing.T) {
	script := scriptForTemplate(t, "gcp-buildpacks-build.yaml", "build-image")
	res := runScript(t, script, map[string]string{
		"podman": buildPodmanStub,
		"pack":   packStub,
		"jq":     buildJQStub,
	}, func(root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol", "source"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
	}, buildReplacements)

	requireScriptExitCode(t, res, 1, "missing app path should fail before pack build")
	requireOutputContains(t, res, "application path",
		"missing app-path failure should explain that the configured application path is invalid")
	requireNoCall(t, res, "pack build",
		"buildpack build must not run pack build when app-path validation fails")
}

func publishReplacements(root string) []string {
	return []string{
		"{{inputs.parameters.git-revision}}", "abcdef12",
		"{{workflow.parameters.image-name}}", "example/app",
		"{{workflow.parameters.image-tag}}", "dev",
		"{{inputs.parameters.image}}", "ignored-by-publish",
		"/mnt/vol", filepath.Join(root, "mnt-vol"),
		"/storage/run", filepath.Join(root, "storage", "run"),
		"/storage/graph", filepath.Join(root, "storage", "graph"),
		"/etc/containers", filepath.Join(root, "containers"),
		"/etc/secrets/registry-push-secret", filepath.Join(root, "registry-push-secret"),
		"/tmp/image.txt", filepath.Join(root, "image.txt"),
	}
}

func TestPublishImage_BehaviorWithAndWithoutAuth(t *testing.T) {
	for _, tc := range []struct {
		name         string
		file         string
		withAuth     bool
		wantRegistry string
		wantTLS      string
	}{
		{"cloud-auth", "publish-image.yaml", true, "ttl.sh/openchoreo-builds", "--tls-verify=true"},
		{"cloud-anonymous", "publish-image.yaml", false, "ttl.sh/openchoreo-builds", "--tls-verify=true"},
		{"k3d-auth", "publish-image-k3d.yaml", true, "host.k3d.internal:10082", "--tls-verify=false"},
		{"k3d-anonymous", "publish-image-k3d.yaml", false, "host.k3d.internal:10082", "--tls-verify=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := echoShim + scriptForTemplate(t, tc.file, "publish-image")
			res := runScript(t, script, map[string]string{"podman": publishPodmanStub}, func(root string) {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, "mnt-vol", "app-image.tar"), []byte("tar"), 0o644))
				if tc.withAuth {
					require.NoError(t, os.MkdirAll(filepath.Join(root, "registry-push-secret"), 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(root, "registry-push-secret", ".dockerconfigjson"), []byte("{}"), 0o600))
				}
			}, publishReplacements)

			requireScriptSuccess(t, res, tc.name+" publish-image script should complete")
			requireCallContains(t, res, "podman push", tc.wantTLS,
				"publish-image must use the profile-specific TLS verification flag")
			requireCallContains(t, res, "podman push", tc.wantRegistry+"/example/app:dev-abcdef12",
				"publish-image must push the registry-qualified image reference")
			if tc.withAuth {
				requireCallContains(t, res, "podman push", "--authfile",
					"publish-image must pass --authfile when registry-push-secret/.dockerconfigjson is mounted")
			} else {
				requireNoCallContains(t, res, "podman push", "--authfile",
					"publish-image must not pass --authfile when registry-push-secret is absent")
			}

			data, err := os.ReadFile(filepath.Join(res.root, "image.txt"))
			if err != nil {
				t.Fatalf(`
contract:
  publish-image must write the final image reference to /tmp/image.txt

expected:
  readable file: $TEST_ROOT/image.txt

actual:
  %v

recorded calls:
%s`,
					err,
					formatCalls(res, res.calls),
				)
			}
			requireEqualOutput(t, res, string(data), tc.wantRegistry+"/example/app:dev-abcdef12",
				"publish-image must write the final image reference to /tmp/image.txt")
		})
	}
}

func TestPublishImage_LoadsAndTagsBuildOutput(t *testing.T) {
	for _, tc := range []struct {
		name         string
		file         string
		wantRegistry string
	}{
		{"cloud", "publish-image.yaml", "ttl.sh/openchoreo-builds"},
		{"k3d", "publish-image-k3d.yaml", "host.k3d.internal:10082"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := echoShim + scriptForTemplate(t, tc.file, "publish-image")
			res := runScript(t, script, map[string]string{"podman": publishPodmanStub}, func(root string) {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
				require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, "mnt-vol", "app-image.tar"), []byte("tar"), 0o644))
			}, publishReplacements)

			requireScriptSuccess(t, res, tc.name+" publish-image script should complete")
			requireCallContains(t, res, "podman load -i", filepath.Join(res.root, "mnt-vol", "app-image.tar"),
				"publish-image must load the build output tar before tagging/pushing")
			requireHasCall(t, res, "podman tag example/app:dev-abcdef12 "+tc.wantRegistry+"/example/app:dev-abcdef12",
				"publish-image must tag the local build image with the target registry endpoint")
		})
	}
}

func TestPublishImage_MissingTarFailsBeforePush(t *testing.T) {
	script := scriptForTemplate(t, "publish-image.yaml", "publish-image")
	res := runScript(t, script, map[string]string{"podman": publishPodmanStub}, func(root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "storage"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "containers"), 0o755))
	}, publishReplacements)

	requireScriptExitCode(t, res, 1, "missing app-image.tar should fail before push")
	requireOutputContains(t, res, "Built image tar not found",
		"missing app-image.tar failure should explain that build output tar is required")
	requireNoCall(t, res, "podman push",
		"publish-image must not push when /mnt/vol/app-image.tar is missing")
}

func callContains(calls []string, prefix, needle string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) && strings.Contains(c, needle) {
			return true
		}
	}
	return false
}

func requireScriptSuccess(t *testing.T, res scriptRunResult, contract string) {
	t.Helper()
	requireScriptExitCode(t, res, 0, contract)
}

func requireScriptExitCode(t *testing.T, res scriptRunResult, want int, contract string) {
	t.Helper()
	if res.exitCode == want {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  exit code: %d

actual:
  exit code: %d

script output:
%s

recorded calls:
%s`,
		contract,
		want,
		res.exitCode,
		formatScriptOutput(res),
		formatCalls(res, res.calls),
	)
}

func requireHasCall(t *testing.T, res scriptRunResult, prefix string, contract string) {
	t.Helper()
	if hasCall(res.calls, prefix) {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  call prefix: %q

recorded calls:
%s`,
		contract,
		normalizeTestPath(res, prefix),
		formatCalls(res, res.calls),
	)
}

func requireNoCall(t *testing.T, res scriptRunResult, prefix string, contract string) {
	t.Helper()
	if !hasCall(res.calls, prefix) {
		return
	}
	t.Fatalf(`
contract:
  %s

unexpected:
  call prefix: %q

matching calls:
%s

all recorded calls:
%s`,
		contract,
		normalizeTestPath(res, prefix),
		formatCalls(res, matchingCalls(res.calls, prefix)),
		formatCalls(res, res.calls),
	)
}

func requireCallContains(t *testing.T, res scriptRunResult, prefix string, needle string, contract string) {
	t.Helper()
	if callContains(res.calls, prefix, needle) {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  call prefix: %q
  must contain: %q

matching calls:
%s

all recorded calls:
%s`,
		contract,
		normalizeTestPath(res, prefix),
		normalizeTestPath(res, needle),
		formatCalls(res, matchingCalls(res.calls, prefix)),
		formatCalls(res, res.calls),
	)
}

func requireNoCallContains(t *testing.T, res scriptRunResult, prefix string, needle string, contract string) {
	t.Helper()
	if !callContains(res.calls, prefix, needle) {
		return
	}
	t.Fatalf(`
contract:
  %s

unexpected:
  call prefix: %q
  contained: %q

matching calls:
%s

all recorded calls:
%s`,
		contract,
		normalizeTestPath(res, prefix),
		normalizeTestPath(res, needle),
		formatCalls(res, matchingCalls(res.calls, prefix)),
		formatCalls(res, res.calls),
	)
}

func requireEqualOutput(t *testing.T, res scriptRunResult, got string, want string, contract string) {
	t.Helper()
	if got == want {
		return
	}
	t.Fatalf(`
contract:
  %s

expected:
  %s

actual:
  %s

recorded calls:
%s`,
		contract,
		normalizeTestPath(res, want),
		normalizeTestPath(res, got),
		formatCalls(res, res.calls),
	)
}

func matchingCalls(calls []string, prefix string) []string {
	var matches []string
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			matches = append(matches, c)
		}
	}
	return matches
}

func formatCalls(res scriptRunResult, calls []string) string {
	if len(calls) == 0 {
		return "  (none)"
	}

	var b strings.Builder
	for _, c := range calls {
		b.WriteString("  - ")
		b.WriteString(normalizeTestPath(res, c))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatScriptOutput(res scriptRunResult) string {
	out := strings.TrimSpace(normalizeTestPath(res, res.output))
	if out == "" {
		return "  (empty)"
	}

	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func normalizeTestPath(res scriptRunResult, s string) string {
	if res.root == "" {
		return s
	}
	return strings.ReplaceAll(s, res.root, "$TEST_ROOT")
}
