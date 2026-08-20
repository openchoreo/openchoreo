#!/usr/bin/env bash
# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
#
# sync-actions-allowlist.sh — allow-list de GitHub Actions para los tres espejos.
#
# EL PROBLEMA QUE RESUELVE
# ------------------------
# Un espejo trae los workflows del upstream, y esos workflows publican imagenes y
# firman releases. Con nuestra identidad y nuestros secretos, eso no puede correr.
# T00 lo resolvio apagando Actions a nivel repositorio, que es efectivo y tambien
# apaga LO NUESTRO: hasta T08b, `idp-upstream-sync.yml` nunca corrio ni una vez, su
# cron era inerte, y ningun PR de los espejos reportaba checks. La regla "sin checks
# = verde" del preambulo de agentes era un parche a esto.
#
# La allow-list real es por workflow, no por repositorio: Actions encendido, y cada
# workflow del upstream apagado por API. No toca un solo archivo del upstream, asi
# que no consume presupuesto de diff ni genera conflicto de rebase.
#
# POR QUE ES RE-EJECUTABLE Y NO UN ONE-SHOT
# -----------------------------------------
# Un workflow nuevo del upstream llega HABILITADO por default en el proximo rebase:
# `disabled_manually` es estado por workflow-id, y un archivo que no existia no puede
# estar apagado. Es una trampa recurrente. Corre esto despues de cada rebase, y en
# seco cuando quieras auditar. El agente de sync (idp-sync/) reporta el drift; esto
# lo corrige.
#
# USO
#   .github/scripts/sync-actions-allowlist.sh --dry-run     # audita, no cambia nada
#   .github/scripts/sync-actions-allowlist.sh               # aplica
#   .github/scripts/sync-actions-allowlist.sh --repo idp-kira-portal
#
# Sale 1 si en --dry-run encuentra drift, para poder usarlo como gate.
#
# REQUISITOS
#   gh autenticado con permiso de administracion sobre la org (scope `repo` clasico
#   alcanza para leer/escribir `actions/permissions` y para disable/enable de
#   workflows). NO hace falta scope `workflow`: no escribe archivos.

set -euo pipefail

ORG="fondomp-production"

# --- Allow-list ---------------------------------------------------------------
# Rutas (no nombres: el `name:` de un workflow lo cambia el upstream sin avisar) de
# los workflows NUESTROS, los unicos que quedan habilitados. Una linea por repo.
#
# Mantener en sync con la lista OWNED de cada `.github/scripts/verify-fork.sh` y con
# la fila 0b de cada PATCHES.md: si un workflow es nuestro, aparece en los tres lados.
allowlist_for() {
  case "$1" in
    idp-openchoreo)
      printf '%s\n' \
        ".github/workflows/idp-upstream-sync.yml" \
        ".github/workflows/idp-fork-ci.yml"
      ;;
    idp-openchoreo-modules)
      printf '%s\n' ".github/workflows/idp-fork-ci.yml"
      ;;
    idp-kira-portal)
      printf '%s\n' ".github/workflows/idp-fork-ci.yml"
      ;;
    *)
      echo "repo sin allow-list declarada: $1" >&2
      return 1
      ;;
  esac
}

REPOS="idp-openchoreo idp-openchoreo-modules idp-kira-portal"

DRY_RUN=0
ONLY_REPO=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --repo) ONLY_REPO="${2:-}"; shift ;;
    -h|--help) sed -n '1,40p' "$0"; exit 0 ;;
    *) echo "argumento desconocido: $1" >&2; exit 2 ;;
  esac
  shift
done
[ -n "$ONLY_REPO" ] && REPOS="$ONLY_REPO"

command -v gh >/dev/null || { echo "gh es requerido" >&2; exit 1; }
: "${GH_HOST:=github.com}"; export GH_HOST

drift=0
say()  { printf '%s\n' "$*"; }
ok()   { printf '  OK    %s\n' "$*"; }
act()  { printf '  FIX   %s\n' "$*"; }
warn() { printf '  DRIFT %s\n' "$*"; drift=1; }

for repo in $REPOS; do
  say "== $ORG/$repo"

  allowed="$(allowlist_for "$repo")"

  # 1. Actions encendido a nivel repositorio.
  enabled="$(gh api "repos/$ORG/$repo/actions/permissions" --jq '.enabled' 2>/dev/null || echo "unknown")"
  if [ "$enabled" != "true" ]; then
    if [ "$DRY_RUN" = 1 ]; then
      warn "Actions deshabilitado a nivel repositorio (nada nuestro puede correr)"
      # Sin Actions encendido la API de workflows devuelve 409; no hay mas que auditar.
      continue
    fi
    # Se enciende y se apagan los del upstream en la misma corrida. La ventana es de
    # segundos y GitHub no reproduce eventos pasados al habilitar, asi que lo unico
    # que podria disparar en el medio es un cron que caiga justo ahi.
    gh api -X PUT "repos/$ORG/$repo/actions/permissions" \
      -F enabled=true -f allowed_actions=all >/dev/null
    act "Actions habilitado"
  else
    ok "Actions habilitado"
  fi

  # 2. Todo workflow que no este en la allow-list queda apagado.
  workflows="$(gh api --paginate "repos/$ORG/$repo/actions/workflows" \
    --jq '.workflows[] | [.id, .state, .path] | @tsv')"

  while IFS=$'\t' read -r id state path; do
    [ -n "${path:-}" ] || continue
    if printf '%s\n' "$allowed" | grep -Fxq -- "$path"; then
      # Nuestro: tiene que estar activo. Un workflow nuestro apagado es tan malo como
      # uno del upstream encendido, y es lo que pasa si alguien lo apaga por la UI.
      if [ "$state" = "active" ]; then
        ok "activo (nuestro)   $path"
      elif [ "$DRY_RUN" = 1 ]; then
        warn "NUESTRO y apagado ($state)   $path"
      else
        gh api -X PUT "repos/$ORG/$repo/actions/workflows/$id/enable" >/dev/null
        act "re-habilitado   $path"
      fi
    else
      if [ "$state" != "active" ]; then
        ok "apagado (upstream) $path"
      elif [ "$DRY_RUN" = 1 ]; then
        warn "UPSTREAM y encendido   $path"
      else
        gh api -X PUT "repos/$ORG/$repo/actions/workflows/$id/disable" >/dev/null
        act "deshabilitado   $path"
      fi
    fi
  done <<EOF
$workflows
EOF

  # 3. Un workflow de la allow-list que no exista es un error de configuracion, no un
  #    "no pasa nada": alguien lo borro o lo renombro y creemos que hay CI donde no hay.
  while IFS= read -r want; do
    [ -n "$want" ] || continue
    if ! printf '%s\n' "$workflows" | cut -f3 | grep -Fxq -- "$want"; then
      warn "declarado en la allow-list y NO existe en el repo: $want"
    fi
  done <<EOF
$allowed
EOF
done

say ""
if [ "$DRY_RUN" = 1 ]; then
  if [ "$drift" = 0 ]; then say "actions-allowlist (dry-run): VERDE"; else say "actions-allowlist (dry-run): DRIFT"; fi
  exit "$drift"
fi
say "actions-allowlist: aplicado"
