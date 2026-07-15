// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowtemplates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const occStub = `#!/bin/sh
args="$*"
echo "occ $args" >> "$CALLS"
if [ "$1" = "version" ]; then
  echo "occ-test"
  exit 0
fi
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    out="$2"
    break
  fi
  shift
done
if [ -n "$out" ]; then
  cat > "$out" <<'YAML'
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: component
spec:
  container:
    image: registry.example.com/app:new
YAML
fi
exit 0
`

const workloadYQStub = `#!/bin/sh
echo "yq $*" >> "$CALLS"
if [ "$1" = "-r" ]; then
  echo "component"
  exit 0
fi
cat <<'JSON'
{"metadata":{"name":"component"},"spec":{"container":{"image":"registry.example.com/app:new"}}}
JSON
exit 0
`

const workloadJQStub = `#!/bin/sh
echo "jq $*" >> "$CALLS"
case "$1" in
  -c)
    cat "$3"
    ;;
  --arg)
    cat <<'JSON'
{"metadata":{"name":"component","labels":{"preserve":"field"}},"spec":{"container":{"image":"registry.example.com/app:new"},"unchanged":"keep"}}
JSON
    ;;
  --rawfile)
    case "$*" in
      *'"true"'*)
        cat <<'JSON'
{"metadata":{"annotations":{"openchoreo.dev/workload":"payload","openchoreo.dev/workload-from-source":"true"}}}
JSON
        ;;
      *)
        cat <<'JSON'
{"metadata":{"annotations":{"openchoreo.dev/workload":"payload","openchoreo.dev/workload-from-source":"false"}}}
JSON
        ;;
    esac
    ;;
  *)
    cat
    ;;
esac
exit 0
`

func workloadCurlStub(createCode int, emptyToken bool) string {
	token := `{"access_token":"token"}`
	if emptyToken {
		token = `{}`
	}
	return fmt.Sprintf(`#!/bin/sh
method="GET"
url=""
data=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -X)
      shift
      method="$1"
      ;;
    -d)
      shift
      data="$1"
      ;;
    -H)
      shift
      echo "curl-header $1" >> "$CALLS"
      ;;
    --data-urlencode)
      shift
      echo "curl-form $1" >> "$CALLS"
      ;;
    http://*|https://*)
      url="$1"
      ;;
  esac
  shift
done
echo "curl $method $url" >> "$CALLS"
if [ -n "$data" ]; then
  path="${data#@}"
  if [ -f "$path" ]; then
    echo "curl-data $(tr -d '\n' < "$path")" >> "$CALLS"
  fi
fi
case "$url" in
  *oauth2/token*)
    printf '%%s\n' '%s'
    ;;
  */api/v1/namespaces/default/workloads)
    printf '{"status":"create"}\n%d\n'
    ;;
  */api/v1/namespaces/default/workloads/component)
    if [ "$method" = "GET" ]; then
      printf '{"metadata":{"name":"component","labels":{"preserve":"field"}},"spec":{"container":{"image":"registry.example.com/app:old"},"unchanged":"keep"}}\n200\n'
    else
      printf '{"status":"updated"}\n200\n'
    fi
    ;;
  */api/v1/namespaces/default/workflowruns/run-1)
    if [ "$method" = "GET" ]; then
      printf '{"metadata":{"annotations":{}}}\n200\n'
    else
      printf '{"status":"annotated"}\n200\n'
    fi
    ;;
  *)
    printf '{"error":"unexpected url"}\n500\n'
    ;;
esac
`, token, createCode)
}

func workloadReplacements(root string, appPath string, scope string) []string {
	return []string{
		"{{inputs.parameters.image}}", "registry.example.com/app:new",
		"{{inputs.parameters.run-name}}", "run-1",
		"{{workflow.parameters.project-name}}", "project",
		"{{workflow.parameters.component-name}}", "component",
		"{{workflow.parameters.namespace-name}}", "default",
		"{{workflow.parameters.app-path}}", appPath,
		"{{inputs.parameters.oauth-token-url}}", "https://auth.example.test/oauth2/token",
		"{{inputs.parameters.oauth-client-id}}", "client-id",
		"{{inputs.parameters.oauth-client-secret}}", "client-secret",
		"{{inputs.parameters.oauth-scope}}", scope,
		"{{inputs.parameters.api-server-url}}", "https://api.example.test",
		"/tmp/openchoreo-workload", filepath.Join(root, "workload-tmp"),
		"/tmp/openchoreo", filepath.Join(root, "home"),
		"/mnt/vol", filepath.Join(root, "mnt-vol"),
		"/tools", filepath.Join(root, "tools"),
	}
}

func TestGenerateWorkload_CreateFromSourceBehavior(t *testing.T) {
	script := scriptForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	env := envForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	res := runScriptWithEnv(t, script, env, map[string]string{
		"occ":  occStub,
		"yq":   workloadYQStub,
		"jq":   workloadJQStub,
		"curl": workloadCurlStub(201, false),
	}, func(root string) {
		appDir := filepath.Join(root, "mnt-vol", "source", "service")
		require.NoError(t, os.MkdirAll(appDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "workload.yaml"), []byte("kind: Workload\n"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "bin"), 0o755))
	}, func(root string) []string {
		return workloadReplacements(root, "service", "scope-a")
	})

	requireHasCall(t, res, "curl POST https://api.example.test/api/v1/namespaces/default/workloads",
		"source-defined workload must assign RESPONSE from curl POST before reading HTTP_CODE")
	requireScriptSuccess(t, res, "source-defined workload should be created through the workload API and annotated on the WorkflowRun")
	requireCallContains(t, res, "occ workload create", "--descriptor workload.yaml",
		"source workload.yaml must be passed to occ with --descriptor")
	requireHasCall(t, res, "yq -o=json del(.apiVersion) | del(.kind)",
		"generated workload payload must strip apiVersion and kind before API submission")
	requireHasCall(t, res, "curl-form scope=scope-a",
		"OAuth token request must include scope when oauth-scope is configured")
	requireCallContains(t, res, "curl-data", `"openchoreo.dev/workload-from-source":"true"`,
		"WorkflowRun annotation must record that the workload came from source")
}

func TestGenerateWorkload_AutoGeneratedConflictMergesOnlyImage(t *testing.T) {
	script := scriptForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	env := envForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	res := runScriptWithEnv(t, script, env, map[string]string{
		"occ":  occStub,
		"yq":   workloadYQStub,
		"jq":   workloadJQStub,
		"curl": workloadCurlStub(409, false),
	}, func(root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol", "source", "service"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "bin"), 0o755))
	}, func(root string) []string {
		return workloadReplacements(root, "service", "")
	})

	requireHasCall(t, res, "curl POST https://api.example.test/api/v1/namespaces/default/workloads",
		"auto-generated workload must assign RESPONSE from curl POST before reading HTTP_CODE")
	requireScriptSuccess(t, res, "auto-generated workload conflict should update only the container image and annotate the WorkflowRun")
	requireHasCall(t, res, "occ workload create",
		"auto-generated workload must still be generated through occ")
	requireNoCallContains(t, res, "occ workload create", "--descriptor workload.yaml",
		"auto-generated workload must not pass --descriptor when workload.yaml is absent")
	requireHasCall(t, res, "curl GET https://api.example.test/api/v1/namespaces/default/workloads/component",
		"409 on auto-generated workload must fetch the existing workload before merge")
	requireHasCall(t, res, "curl PUT https://api.example.test/api/v1/namespaces/default/workloads/component",
		"409 on auto-generated workload must PUT the merged workload")
	requireCallContains(t, res, "curl-data", `"preserve":"field"`,
		"auto-generated workload merge must preserve existing non-image fields")
	requireCallContains(t, res, "curl-data", `"image":"registry.example.com/app:new"`,
		"auto-generated workload merge must update only the container image")
	requireCallContains(t, res, "curl-data", `"openchoreo.dev/workload-from-source":"false"`,
		"WorkflowRun annotation must record that the workload was auto-generated")
}

func TestGenerateWorkload_EmptyOAuthTokenFailsBeforeCreate(t *testing.T) {
	script := scriptForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	env := envForTemplate(t, "generate-workload.yaml", "generate-workload-cr")
	res := runScriptWithEnv(t, script, env, map[string]string{
		"occ":  occStub,
		"yq":   workloadYQStub,
		"jq":   workloadJQStub,
		"curl": workloadCurlStub(201, true),
	}, func(root string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "mnt-vol", "source", "service"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "bin"), 0o755))
	}, func(root string) []string {
		return workloadReplacements(root, "service", "")
	})

	requireScriptExitCode(t, res, 1, "empty OAuth access_token should fail before calling the workload API")
	requireOutputContains(t, res, "Failed to get access token",
		"empty OAuth access_token failure should explain that token acquisition failed")
	requireNoCallContains(t, res, "curl POST", "/api/v1/namespaces/default/workloads",
		"workload API must not be called when OAuth token acquisition fails")
}

func requireOutputContains(t *testing.T, res scriptRunResult, needle string, contract string) {
	t.Helper()
	if strings.Contains(res.output, needle) {
		return
	}
	t.Fatalf(`
contract:
  %s

expected output to contain:
  %q

script output:
%s

recorded calls:
%s`,
		contract,
		needle,
		formatScriptOutput(res),
		formatCalls(res, res.calls),
	)
}
