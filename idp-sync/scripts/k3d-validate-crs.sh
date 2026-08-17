#!/usr/bin/env bash
# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
#
# Levanta un k3d efimero con los CRDs y los webhooks del tag NUEVO, y valida contra el
# API server todos los CRs de `idp-platform` con `--dry-run=server`.
#
# Por que un cluster de verdad y no `kubeconform`: la mitad de las reglas que nos pueden
# romper no estan en el schema. Viven en los webhooks del control plane y en las
# `preRenderValidations` CEL de ComponentTypes y Traits. Un validador offline las ignora
# y da verde.
#
# Emite un JSON en --out. Nunca escribe secretos ni kubeconfigs fuera de $TMPDIR.

set -euo pipefail

CHART_DIR=""
CRD_DIR=""
VALUES_FILE=""
OUT="k3d-result.json"
CLUSTER_NAME="idp-sync-$(date +%s)"
CERT_MANAGER_VERSION="v1.16.2"
declare -a CR_DIRS=()

usage() {
  cat <<'EOF'
Uso: k3d-validate-crs.sh --chart-dir DIR --crd-dir DIR [--values FILE] [--cr-dir DIR]... --out FILE

  --chart-dir  Chart del control plane en el tag NUEVO (trae los webhooks).
  --crd-dir    Directorio con los CRDs del tag NUEVO (config/crd/bases).
  --values     Values propios (idp-platform). Opcional.
  --cr-dir     Directorio con CRs a validar. Repetible. Sin ninguno, el check se saltea.
  --out        Archivo JSON de salida.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --chart-dir) CHART_DIR="$2"; shift 2 ;;
    --crd-dir)   CRD_DIR="$2";   shift 2 ;;
    --values)    VALUES_FILE="$2"; shift 2 ;;
    --cr-dir)    CR_DIRS+=("$2"); shift 2 ;;
    --out)       OUT="$2";       shift 2 ;;
    --cluster)   CLUSTER_NAME="$2"; shift 2 ;;
    -h|--help)   usage; exit 0 ;;
    *) echo "argumento desconocido: $1" >&2; usage; exit 2 ;;
  esac
done

[[ -n "$CHART_DIR" && -n "$CRD_DIR" ]] || { usage; exit 2; }

WORK="$(mktemp -d)"
export KUBECONFIG="$WORK/kubeconfig"
RESULTS="$WORK/results.jsonl"
: >"$RESULTS"

STATUS_CLUSTER="false"
STATUS_CRDS="false"
STATUS_WEBHOOKS="false"
NOTE=""

cleanup() {
  local rc=$?
  k3d cluster delete "$CLUSTER_NAME" >/dev/null 2>&1 || true
  # El heredoc va al final: todas las palabras previas son argv de python3.
  python3 - "$OUT" "$RESULTS" "$STATUS_CLUSTER" "$STATUS_CRDS" "$STATUS_WEBHOOKS" "$NOTE" <<'PY'
import json, sys, pathlib
out, results, cluster, crds, webhooks, note = sys.argv[1:7]
items = []
path = pathlib.Path(results)
if path.is_file():
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line:
            items.append(json.loads(line))
pathlib.Path(out).write_text(json.dumps({
    "cluster_ok": cluster == "true",
    "crds_ok": crds == "true",
    "webhooks_ok": webhooks == "true",
    "note": note,
    "crs": items,
}, indent=2, ensure_ascii=False), encoding="utf-8")
PY
  rm -rf "$WORK"
  exit "$rc"
}
trap cleanup EXIT

log() { echo "::group::$*"; }
endlog() { echo "::endgroup::"; }

# ── 1. Cluster efimero ────────────────────────────────────────────────────────────────
log "k3d: creando $CLUSTER_NAME"
k3d cluster create "$CLUSTER_NAME" \
  --no-lb \
  --k3s-arg "--disable=traefik@server:0" \
  --k3s-arg "--disable=metrics-server@server:0" \
  --wait --timeout 300s
kubectl cluster-info >/dev/null
STATUS_CLUSTER="true"
endlog

# ── 2. cert-manager: los webhooks del control plane no arrancan sin certificados ──────
log "cert-manager $CERT_MANAGER_VERSION"
helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --version "$CERT_MANAGER_VERSION" \
  --set crds.enabled=true \
  --wait --timeout 10m
endlog

# ── 3. CRDs del tag nuevo ────────────────────────────────────────────────────────────
# `--server-side --force-conflicts` es el mismo comando que hay que correr a mano en el
# upgrade real, porque `helm upgrade` no toca `crds/`. Si falla aca, falla alla.
log "CRDs desde $CRD_DIR"
kubectl apply --server-side --force-conflicts -f "$CRD_DIR"
kubectl wait --for=condition=Established --timeout=180s crd --all
STATUS_CRDS="true"
endlog

# ── 4. Control plane (por los webhooks) ──────────────────────────────────────────────
log "control plane desde $CHART_DIR"
helm_args=(upgrade --install idp "$CHART_DIR" --namespace idp-control-plane --create-namespace --wait --timeout 15m)
[[ -n "$VALUES_FILE" ]] && helm_args+=(-f "$VALUES_FILE")
# Sin `set -e` en esta rama: que el control plane no levante es un hallazgo, no un crash.
if helm "${helm_args[@]}" >/dev/null 2>"$WORK/helm.err"; then
  STATUS_WEBHOOKS="true"
else
  NOTE="El control plane no llego a Ready: $(tail -c 800 "$WORK/helm.err" | tr '\n' ' ')"
  echo "::warning::$NOTE"
fi
endlog

# ── 5. Revalidacion de los CRs ───────────────────────────────────────────────────────
if [[ ${#CR_DIRS[@]} -eq 0 ]]; then
  NOTE="${NOTE:+$NOTE | }No se pasaron directorios de CRs."
  exit 0
fi

log "dry-run de los CRs"
for dir in "${CR_DIRS[@]}"; do
  [[ -d "$dir" ]] || continue
  while IFS= read -r -d '' file; do
    if err="$(kubectl apply --server-side --force-conflicts --dry-run=server -f "$file" 2>&1 >/dev/null)"; then
      python3 -c 'import json,sys; print(json.dumps({"file": sys.argv[1], "ok": True, "error": ""}))' "$file" >>"$RESULTS"
    else
      python3 -c 'import json,sys; print(json.dumps({"file": sys.argv[1], "ok": False, "error": sys.argv[2][:2000]}))' "$file" "$err" >>"$RESULTS"
    fi
  done < <(find "$dir" -type f \( -name '*.yaml' -o -name '*.yml' \) -print0)
done
endlog
