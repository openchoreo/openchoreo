#!/usr/bin/env bash
# Local verification for the declarative workflow results branch (PR #4696).
#
# Brings up a k3d e2e cluster, replaces the published controller and API images
# with builds from the current working tree, and runs the build + UI suites
# against them.
#
# It encodes several traps that are easy to hit and that fail misleadingly:
#
#   * The chart sets imagePullPolicy: Always. Importing an image and restarting
#     re-pulls latest-dev from ghcr and silently runs main's build instead. Worse,
#     once that happens the tag in containerd points at the pulled image, so
#     switching the policy back does not undo it - you must re-import.
#   * Any helm upgrade (including `make e2e.setup-ui`) resets the pull policy and
#     undoes the swap. swap_images is therefore called again after UI setup.
#   * yq is required by `make e2e.setup` and is not in any documented prereq list.
#   * The occ binary is required by the UI pkce-login spec.
#   * The build-logs-api spec needs *.e2e-cp.local in /etc/hosts.
#
# Usage:
#   ./verify-local.sh              # everything
#   ./verify-local.sh setup        # cluster + images only
#   ./verify-local.sh build        # build e2e suite only (assumes setup done)
#   ./verify-local.sh ui           # UI suite only (assumes setup done)
#   ./verify-local.sh images       # re-swap images (run after any helm upgrade)
#   ./verify-local.sh demo         # seed a real component + build (for looking at the UI)
#   ./verify-local.sh demo-clean   # remove the demo component
#   ./verify-local.sh url          # print access instructions
#   ./verify-local.sh teardown     # delete the cluster
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CLUSTER="${E2E_CLUSTER_NAME:-openchoreo-e2e}"
CTX="k3d-${CLUSTER}"
CP_NS=openchoreo-control-plane
YQ_VERSION=v4.45.4
YQ_SHA256=b96de04645707e14a12f52c37e6266832e03c29e95b9b139cddcae7314466e69
TOOL_BIN="$REPO_ROOT/bin/tools"
export PATH="$TOOL_BIN:/usr/local/go/bin:$PATH"

log()  { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
die()  { printf '\n\033[1;31mFAIL: %s\033[0m\n' "$*" >&2; exit 1; }

preflight() {
  log "Preflight"
  for t in docker k3d kubectl helm go node; do
    command -v "$t" >/dev/null 2>&1 || die "$t is not on PATH"
  done
  docker info >/dev/null 2>&1 || die "docker daemon is not reachable"

  # yq: make e2e.setup fails at _e2e.configure-dp without it, with a bare
  # "yq: command not found" that does not say which step needed it.
  if ! command -v yq >/dev/null 2>&1; then
    log "Installing yq ${YQ_VERSION}"
    mkdir -p "$TOOL_BIN"
    curl -fsSL -o "$TOOL_BIN/yq" \
      "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64"
    echo "${YQ_SHA256}  ${TOOL_BIN}/yq" | sha256sum -c - >/dev/null \
      || die "yq checksum mismatch"
    chmod +x "$TOOL_BIN/yq"
  fi

  # occ: the UI pkce-login spec spawns it and reports only
  # "controlplane add exited null: undefined" when it is missing.
  [ -x bin/dist/linux/amd64/occ ] || { log "Building occ"; make go.build.occ >/dev/null; }

  # The build-logs-api spec and any host-side API call resolve these names.
  if ! grep -q "openchoreo.e2e-cp.local" /etc/hosts 2>/dev/null; then
    log "Adding *.e2e-cp.local to /etc/hosts (needs sudo)"
    echo '127.0.0.1 api.e2e-cp.local thunder.e2e-cp.local openchoreo.e2e-cp.local' \
      | sudo tee -a /etc/hosts >/dev/null
  fi
}

# Replace the published images with builds from the working tree, then prove by
# digest that the cluster is running them. A locally imported image has an
# imageID of "sha256:..."; one pulled from a registry has "ghcr.io/...@sha256:...".
swap_images() {
  log "Building controller and openchoreo-api from the working tree"
  make k3d.build.controller k3d.build.openchoreo-api >/dev/null

  log "Importing into ${CLUSTER}"
  k3d image import \
    ghcr.io/openchoreo/controller:latest-dev \
    ghcr.io/openchoreo/openchoreo-api:latest-dev \
    --cluster "$CLUSTER" >/dev/null

  for d in controller-manager openchoreo-api; do
    kubectl --context "$CTX" -n "$CP_NS" patch deployment "$d" --type=json \
      -p='[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]' >/dev/null
    kubectl --context "$CTX" -n "$CP_NS" rollout restart "deployment/$d" >/dev/null
  done
  for d in controller-manager openchoreo-api; do
    kubectl --context "$CTX" -n "$CP_NS" rollout status "deployment/$d" --timeout=300s >/dev/null
  done

  verify_images
}

# Fails loudly rather than letting the suites run against main's build, which is
# what makes a wrong-image run look like a code defect.
#
# Caveat: this proves the image was imported locally, not that it was built from
# the current tree. docker's image ID and containerd's imageID differ for the same
# image, so they cannot be compared directly; a stale local build would pass this
# check. swap_images always rebuilds before importing, so run it after changing code.
verify_images() {
  log "Verifying the cluster runs the local builds"
  for i in $(seq 1 40); do
    pulled=$(kubectl --context "$CTX" -n "$CP_NS" get pods -o json \
      | python3 -c '
import json,sys
d=json.load(sys.stdin); n=0
for p in d["items"]:
    nm=p["metadata"]["name"]
    if ("controller-manager" in nm or "openchoreo-api" in nm) and p["status"].get("phase")=="Running":
        for c in p["status"].get("containerStatuses",[]):
            if not c["imageID"].startswith("sha256:"): n+=1
print(n)')
    [ "$pulled" = "0" ] && break
    sleep 5   # an old pod may still be terminating; do not judge mid-rollout
  done
  [ "$pulled" = "0" ] || die "a controller/api pod is running a registry-pulled image, not the local build"
  kubectl --context "$CTX" -n "$CP_NS" get pods -o json | python3 -c '
import json,sys
d=json.load(sys.stdin)
for p in d["items"]:
    nm=p["metadata"]["name"]
    if ("controller-manager" in nm or "openchoreo-api" in nm) and p["status"].get("phase")=="Running":
        for c in p["status"].get("containerStatuses",[]):
            print("    %-22s %s" % (nm.rsplit("-",2)[0], c["imageID"][:26]+"...  (local build)"))'
}

setup() {
  preflight
  log "Creating the e2e cluster with the workflow plane (this is the slow part)"
  make e2e.setup E2E_WITH_BUILD=true E2E_SETUP_TIMEOUT=25m

  log "Enabling Backstage"
  # The post-install gateway patch is not idempotent on an existing cluster and
  # can exit non-zero ("Duplicate value: tmp") after the helm upgrade succeeded.
  make e2e.setup-ui E2E_SETUP_TIMEOUT=20m || \
    log "setup-ui returned non-zero; continuing if Backstage is Running (see note above)"
  kubectl --context "$CTX" -n "$CP_NS" rollout status deployment/backstage --timeout=600s

  # Must come last: the helm upgrade above resets imagePullPolicy and undoes the swap.
  swap_images
}

run_build_suite() {
  verify_images
  log "Running the tier3 build suite"
  go test ./test/e2e/suites/build/ -v -ginkgo.v -timeout 90m \
    --e2e.kubecontext="$CTX" --ginkgo.label-filter=tier3
}

run_ui_suite() {
  verify_images
  log "Running the Backstage UI suite"
  ( cd test/ui
    [ -d node_modules ] || npm ci
    npx playwright install --with-deps chromium
    UI_BASE_URL=http://openchoreo.e2e-cp.local:28080 npx playwright test --reporter=line )
}

# Seed a component that actually builds, for eyeballing the UI. The e2e suite
# installs Gitea and mirrors the sample repo into it for CI hermeticity, but the
# workflow plane has outbound network, so checkout-source clones the public URL
# directly - no Gitea, no mirroring.
#
# Two things that bite here:
#   * componentType.name is "deployment/service", not "service". The bare name is
#     rejected by a pattern that does not mention the expected prefix.
#   * The workflow must appear in the component type's allowedWorkflows, or the run
#     fails with ComponentValidationFailed before it ever reaches Argo.
DEMO_NS=default
DEMO_COMPONENT=greeter-demo
DEMO_REPO=https://github.com/openchoreo/sample-workloads.git
DEMO_APP_PATH=/service-go-greeter

demo() {
  local k="kubectl --context $CTX"
  log "Seeding component ${DEMO_COMPONENT} in project ${DEMO_NS}"

  local params
  params=$(cat <<JSON
      repository:
        url: ${DEMO_REPO}
        appPath: ${DEMO_APP_PATH}
      docker:
        context: ${DEMO_APP_PATH}
        filePath: ${DEMO_APP_PATH}/Dockerfile
JSON
)

  $k apply -f - >/dev/null <<YAML
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: ${DEMO_COMPONENT}
  namespace: ${DEMO_NS}
  labels:
    openchoreo.dev/name: ${DEMO_COMPONENT}
    openchoreo.dev/project: ${DEMO_NS}
    openchoreo.dev/component: ${DEMO_COMPONENT}
spec:
  owner:
    projectName: ${DEMO_NS}
  componentType:
    kind: ClusterComponentType
    name: deployment/service
  autoDeploy: true
  workflow:
    kind: ClusterWorkflow
    name: dockerfile-builder
    parameters:
${params}
YAML

  local run="${DEMO_COMPONENT}-run-$(date +%H%M%S)"
  log "Triggering build ${run} (podman build; several minutes)"
  $k apply -f - >/dev/null <<YAML
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: ${run}
  namespace: ${DEMO_NS}
  labels:
    openchoreo.dev/project: ${DEMO_NS}
    openchoreo.dev/component: ${DEMO_COMPONENT}
spec:
  workflow:
    kind: ClusterWorkflow
    name: dockerfile-builder
    parameters:
${params}
YAML

  for _ in $(seq 1 90); do
    [ "$($k -n "$DEMO_NS" get workflowrun "$run" \
          -o jsonpath='{.status.conditions[?(@.type=="WorkflowCompleted")].status}' 2>/dev/null)" = "True" ] && break
    sleep 20
  done
  sleep 8

  log "Build outcome"
  $k -n "$DEMO_NS" get workflowrun "$run" -o jsonpath='{range .status.tasks[*]}    {.name}={.phase}{"\n"}{end}'
  echo "    --- status.results ---"
  $k -n "$DEMO_NS" get workflowrun "$run" -o jsonpath='{range .status.results[*]}    {.name} = {.value}{"\n"}{end}'
  echo "    --- downstream ---"
  $k -n "$DEMO_NS" get workload,componentrelease,releasebinding --no-headers 2>/dev/null | awk '{print "    "$1}'
  $k -n "$DEMO_NS" get component "$DEMO_COMPONENT" \
    -o jsonpath='{range .status.conditions[*]}    component {.type}={.status} ({.reason}){"\n"}{end}'
}

demo_clean() {
  local k="kubectl --context $CTX"
  log "Removing the demo component and its runs"
  $k -n "$DEMO_NS" delete component "$DEMO_COMPONENT" --ignore-not-found >/dev/null
  $k -n "$DEMO_NS" delete workflowrun -l "openchoreo.dev/component=${DEMO_COMPONENT}" --ignore-not-found >/dev/null
}

url() {
  cat <<'EOF'

  Backstage is served on this host only. It is plain HTTP, it shares a port with
  the control-plane API, and the IdP users below are published in the repo - so do
  not open a firewall to it. Tunnel instead:

      ssh -L 28080:localhost:28080 <user>@<this-host>

  Add to the /etc/hosts of the machine you browse from (the gateway routes on the
  Host header, so the bare IP will not work):

      127.0.0.1 openchoreo.e2e-cp.local api.e2e-cp.local thunder.e2e-cp.local

      URL:  http://openchoreo.e2e-cp.local:28080

      platform-engineer@openchoreo.dev / PE@123
      developer@openchoreo.dev         / Dev@123

  Read the feature's own output directly:

      TOKEN=$(curl -s -X POST http://thunder.e2e-cp.local:28080/oauth2/token \
        -u service_mcp_client:service_mcp_client_secret \
        -d grant_type=client_credentials | jq -r .access_token)
      curl -s -H "Authorization: Bearer $TOKEN" \
        http://api.e2e-cp.local:28080/api/v1/namespaces/<ns>/workflowruns/<run> | jq '.status.results, .status.testReport'
EOF
}

case "${1:-all}" in
  all)      setup; run_build_suite; run_ui_suite; url ;;
  setup)    setup; url ;;
  images)   swap_images ;;
  build)    run_build_suite ;;
  ui)       run_ui_suite ;;
  demo)       demo; url ;;
  demo-clean) demo_clean ;;
  url)      url ;;
  teardown) log "Deleting ${CLUSTER}"; k3d cluster delete "$CLUSTER" ;;
  *)        die "unknown command: $1 (all|setup|images|build|ui|demo|demo-clean|url|teardown)" ;;
esac
