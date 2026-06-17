#!/usr/bin/env bash
# Render each plane chart against the bundled install value files (k3d + quick-start).

set -euo pipefail

if ! command -v helm >/dev/null 2>&1; then
  echo "helm is required but not found on PATH" >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

CHARTS_DIR="install/helm"

chart_for_suffix() {
  case "$1" in
    cp) echo "openchoreo-control-plane" ;;
    dp) echo "openchoreo-data-plane" ;;
    op) echo "openchoreo-observability-plane" ;;
    wp) echo "openchoreo-workflow-plane" ;;
    *)  echo "" ;;
  esac
}

VALUE_DIRS=(
  "install/k3d/single-cluster"
  "install/k3d/multi-cluster"
  "install/quick-start"
)

# Register dependency repos, then vendor dependencies for charts that have them.
repo_idx=0
for chart in "$CHARTS_DIR"/*/; do
  grep -qs '^dependencies:' "${chart}Chart.yaml" || continue
  while read -r repo_url; do
    helm repo add "render-test-$repo_idx" "$repo_url" >/dev/null 2>&1 || true
    repo_idx=$((repo_idx + 1))
  done < <(grep -E '^[[:space:]]*repository:[[:space:]]*"?https?://' "${chart}Chart.yaml" \
    | sed -E 's/.*repository:[[:space:]]*"?([^"[:space:]]+).*/\1/')
  helm dependency build "$chart" >/dev/null
done

failures=0
checked=0
for dir in "${VALUE_DIRS[@]}"; do
  for values in "$dir"/values-*.yaml "$dir"/.values-*.yaml; do
    [ -f "$values" ] || continue
    suffix="$(basename "$values" .yaml)"
    suffix="${suffix##*-}"
    chart_name="$(chart_for_suffix "$suffix")"
    [ -n "$chart_name" ] || continue

    checked=$((checked + 1))
    if output="$(helm template render-test "$CHARTS_DIR/$chart_name" -f "$values" 2>&1)"; then
      echo "PASS  $values -> $chart_name"
    else
      echo "FAIL  $values -> $chart_name"
      echo "$output" | sed 's/^/      /'
      failures=$((failures + 1))
    fi
  done
done

echo
if [ "$failures" -ne 0 ]; then
  echo "$failures of $checked render(s) failed."
  exit 1
fi
echo "All $checked render(s) passed."
