#!/usr/bin/env bash
# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
#
# verify-fork.sh — verificacion minima del espejo. Sin dependencias: bash + git.
#
# Contesta cuatro preguntas, que son los cuatro modos de falla que ya nos mordieron:
#
#   1. El sha del pin que dice PATCHES.md, ¿es el mismo al que apunta `upstream-main`?
#   2. Ese sha, ¿es un COMMIT o el objeto de un tag anotado?
#      (`git rev-parse <tag>` a secas devuelve el objeto tag: fue el bug reportado por T08.)
#   3. Si el pin es un tag, ¿`<tag>^{commit}` coincide con el sha registrado?
#   4. Todo archivo con diff contra `upstream-main`, ¿tiene fila en «Parches vigentes»?
#      PATCHES.md enuncia esa regla; sin este check nadie la hace cumplir.
#
# Uso:  .github/scripts/verify-fork.sh [rama-main] [rama-espejo]
#       .github/scripts/verify-fork.sh --self-test     # ¿el verificador detecta lo que dice?
#
# Sale 1 si algo falla. Los SKIP (falta el tag localmente) no fallan, se reportan.
# PATCHES_FILE permite apuntar a una copia — lo usa --self-test.

set -euo pipefail

# --- Configuracion del espejo -------------------------------------------------
# Listas separadas por espacios (no arrays: bash 3.2 de macOS + `set -u` no se llevan
# bien con `${#arr[@]}` sobre un array vacio).
#
PINNED_TAG="v1.2.2"                              # el pin de este espejo SI es un tag
REQUIRED_AT_PIN="config/crd/bases"               # la superficie mas peligrosa del bump
REQUIRED_CONTENT_FILE=""
REQUIRED_CONTENT_NEEDLE=""
# Archivos nuestros por construccion: no son parches sobre archivos del upstream.
# Un item terminado en "/" cubre todo el subarbol. `idp-sync/` es el agente de
# sincronizacion (T08); su upstream.json lleva la misma lista.
OWNED=".github/scripts/verify-fork.sh idp-sync/ .github/workflows/idp-upstream-sync.yml"

# Forma que tiene que conservar el ClusterRole del cluster-agent (check 6).
# El agente aplica dentro de Cells creadas en runtime (dp-<ns>-<proj>-<env>-<hash>),
# asi que estos permisos NO pueden vivir en un Role namespaced. T12 los movio a un
# Role y dejo la plataforma sin poder desplegar nada; lo encontro T73 con el primer
# workload real. Este check existe para que un rebase no lo reintroduzca en silencio.
AGENT_CLUSTERROLES="install/helm/openchoreo-data-plane/templates/cluster-agent/clusterrole.yaml install/helm/openchoreo-workflow-plane/templates/cluster-agent/clusterrole.yaml"
# Kinds sin los cuales no se puede materializar una Cell ni desplegar un componente.
AGENT_REQUIRED_KINDS="deployments services configmaps secrets networkpolicies"
# Los emite ClusterProjectType/idp-default (T28). Upstream nunca los otorgo.
AGENT_REQUIRED_KINDS_DATAPLANE="resourcequotas limitranges"
# -----------------------------------------------------------------------------

fail=0
say()  { printf '%s\n' "$*"; }
ok()   { printf '  OK    %s\n' "$*"; }
bad()  { printf '  FAIL  %s\n' "$*"; fail=1; }
skip() { printf '  SKIP  %s\n' "$*"; }

cd "$(git rev-parse --show-toplevel)"

# check_agent_clusterrole <ruta> <contenido> — usada por el check 6 y por su self-test.
# Toma el contenido por parametro y no por ruta para que el self-test pueda pasarle una
# mutacion sin tocar el arbol ni el indice de git.
check_agent_clusterrole() {
  _cr="$1"; _body="$2"

  _required="$AGENT_REQUIRED_KINDS"
  case "$_cr" in *openchoreo-data-plane*) _required="$_required $AGENT_REQUIRED_KINDS_DATAPLANE" ;; esac

  _missing=""
  for _kind in $_required; do
    printf '%s\n' "$_body" | grep -Eq "^[[:space:]]*-[[:space:]]+${_kind}[[:space:]]*$" || _missing="$_missing $_kind"
  done
  if [ -n "$_missing" ]; then
    bad "$_cr no otorga:$_missing (¿volvieron a un Role namespaced? las Cells se crean en runtime)"
  else
    ok "$_cr otorga los kinds de una Cell"
  fi

  # Verbos del bloque `- apiGroups: ["rbac.authorization.k8s.io"]`, hasta el proximo
  # `- apiGroups:`. Sin leer los verbos solo se podria afirmar que la cadena aparece.
  _rbac_verbs=$(printf '%s\n' "$_body" | awk '
    /^- apiGroups:/ { inblock = ($0 ~ /rbac\.authorization\.k8s\.io/) }
    inblock && /^[[:space:]]*verbs:/ { print; inblock = 0 }
  ')
  if [ -z "$_rbac_verbs" ]; then
    # Sin lectura de RBAC, el read-back de RenderedRelease aborta con 403 y TODO
    # componente queda NotReady para siempre, con el pod 1/1 sirviendo. Rojo, no skip.
    bad "$_cr no otorga lectura de rbac.authorization.k8s.io (el status de todo componente queda NotReady)"
  elif printf '%s\n' "$_rbac_verbs" | grep -Eq '"\*"|create|update|patch|delete|bind|escalate|deletecollection'; then
    bad "$_cr otorga escritura sobre RBAC ($_rbac_verbs) — es la escalacion que cerro T12"
  else
    ok "$_cr lee RBAC sin poder escribirlo ($_rbac_verbs)"
  fi
}

# Resolver las dos ramas UNA vez. En un clon fresco la rama del espejo puede existir
# solo como `origin/upstream-main`; sin esto los checks 4 y 5 revientan con "ambiguous
# argument" y el self-test lee ese crash como si hubiera detectado el fallo inyectado.
resolve() {
  git rev-parse -q --verify "refs/heads/$1^{commit}" 2>/dev/null && return 0
  git rev-parse -q --verify "refs/remotes/origin/$1^{commit}" 2>/dev/null && return 0
  git rev-parse -q --verify "$1^{commit}" 2>/dev/null && return 0
  return 1
}

# --- Self-test ----------------------------------------------------------------
# Un verificador que nunca vio un rojo no es un verificador. Estas dos mutaciones
# son exactamente los dos hallazgos de T08; si el script no las detecta, miente.
# (La segunda encontro un bug real: leer TODA la fila en vez de solo la columna
# «Archivos» hacia que una mencion en la prosa de «Motivo» apagara el hallazgo.)
if [ "${1:-}" = "--self-test" ]; then
  tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT

  say "== self-test 1: un sha de objeto tag en vez del commit debe dar ROJO"
  tag_obj=$(git for-each-ref --format='%(objecttype) %(objectname)' refs/tags \
    | awk '$1=="tag"{print $2; exit}')
  if [ -z "$tag_obj" ]; then
    skip "no hay tags anotados en este clon (corre: git fetch upstream --tags)"
  else
    sed "s/[0-9a-f]\{40\}/$tag_obj/" PATCHES.md > "$tmp"
    if PATCHES_FILE="$tmp" "$0" >/dev/null 2>&1; then
      bad "el sha de un objeto tag paso como valido"
    else ok "detectado"; fi
  fi

  say "== self-test 2: borrar las rutas de la columna «Archivos» debe dar ROJO"
  if git diff --quiet "$(resolve "${3:-upstream-main}")" "$(resolve "${2:-main}")"; then
    skip "no hay diff contra el espejo: nada que dejar sin declarar"
  else
    # Saca los backticks de las filas de datos de la tabla: sin ellos, ninguna ruta
    # queda declarada y todo archivo con diff tiene que reportarse.
    awk -F'|' '/^\|/ && $2 ~ /^[[:space:]]*[0-9]/ { gsub(/`/, "") } { print }' \
      PATCHES.md > "$tmp"
    if PATCHES_FILE="$tmp" "$0" >/dev/null 2>&1; then
      bad "un archivo con diff y sin fila paso como valido"
    else ok "detectado"; fi
  fi

  # Los dos modos de falla del check 6, inyectados sobre el archivo real. Son
  # exactamente las dos formas que tuvo el parche de T12 antes de T73/T12b.
  DP_CR="install/helm/openchoreo-data-plane/templates/cluster-agent/clusterrole.yaml"
  if [ ! -f "$DP_CR" ]; then
    say "== self-test 3 y 4: SKIP"
    skip "no existe $DP_CR en este arbol"
  else
    say "== self-test 3: mover los permisos de Cell a un Role debe dar ROJO"
    # La forma de T12: el ClusterRole se queda solo con recursos cluster-scoped.
    mutated=$(awk '/^- apiGroups: \["apps"\]/{exit} {print}' "$DP_CR")
    before="$fail"; fail=0
    check_agent_clusterrole "$DP_CR" "$mutated" >/dev/null 2>&1
    if [ "$fail" = 0 ]; then fail="$before"; bad "un ClusterRole sin los kinds de una Cell paso como valido"
    else fail="$before"; ok "detectado"; fi

    say "== self-test 4: devolver escritura sobre RBAC debe dar ROJO"
    mutated=$(sed 's/^  verbs: \["get", "list", "watch"\]$/  verbs: ["*"]/' "$DP_CR")
    before="$fail"; fail=0
    check_agent_clusterrole "$DP_CR" "$mutated" >/dev/null 2>&1
    if [ "$fail" = 0 ]; then fail="$before"; bad "un ClusterRole con verbo '*' sobre RBAC paso como valido"
    else fail="$before"; ok "detectado"; fi
  fi

  say ""
  if [ "$fail" = 0 ]; then say "self-test: VERDE"; else say "self-test: ROJO"; fi
  exit "$fail"
fi
# -----------------------------------------------------------------------------

MAIN_REF="${1:-main}"
MIRROR_REF="${2:-upstream-main}"
PATCHES="${PATCHES_FILE:-PATCHES.md}"

[ -f "$PATCHES" ] || { say "no existe $PATCHES en la raiz del repo"; exit 1; }

MAIN_SHA=$(resolve "$MAIN_REF")   || { say "no pude resolver la rama '$MAIN_REF'"; exit 1; }
MIRROR_SHA=$(resolve "$MIRROR_REF") || { say "no pude resolver la rama '$MIRROR_REF'"; exit 1; }

# El sha registrado: fila "| Commit | `<sha>` ... |" de la tabla de cabecera.
declared_sha=$(
  sed -n 's/^|[[:space:]]*Commit[[:space:]]*|[[:space:]]*`\([0-9a-f]\{40\}\)`.*/\1/p' \
    "$PATCHES" | head -n1
)

say "== 1. El pin de $PATCHES coincide con $MIRROR_REF"
if [ -z "$declared_sha" ]; then
  bad "no encontre un sha de 40 caracteres en la fila 'Commit' de $PATCHES"
else
  if [ "$declared_sha" = "$MIRROR_SHA" ]; then
    ok "$declared_sha"
  else
    bad "$PATCHES dice $declared_sha; $MIRROR_REF esta en $MIRROR_SHA"
  fi
fi

say "== 2. El sha registrado es un commit, no el objeto de un tag anotado"
if [ -n "$declared_sha" ]; then
  t=$(git cat-file -t "$declared_sha" 2>/dev/null || echo "ausente")
  case "$t" in
    commit) ok "git cat-file -t -> commit" ;;
    ausente) skip "el objeto $declared_sha no esta en este clon" ;;
    *) bad "el sha registrado es un objeto '$t'. Usa: git rev-parse '<tag>^{commit}'" ;;
  esac
fi

say "== 3. El tag fijado resuelve al sha registrado"
if [ -z "$PINNED_TAG" ]; then
  skip "este espejo se pinnea a un commit de main (excepcion D9.0), no a un tag"
elif ! git rev-parse -q --verify "refs/tags/$PINNED_TAG" >/dev/null; then
  skip "el tag $PINNED_TAG no esta en este clon (corre: git fetch upstream --tags)"
else
  tag_commit=$(git rev-parse "$PINNED_TAG^{commit}")
  if [ "$tag_commit" = "$declared_sha" ]; then
    ok "$PINNED_TAG^{commit} = $tag_commit"
  else
    bad "$PINNED_TAG^{commit} es $tag_commit, pero $PATCHES dice $declared_sha"
  fi
fi

say "== 4. Todo archivo con diff vs $MIRROR_REF esta declarado en «Parches vigentes»"
# SOLO la columna «Archivos» de la tabla de parches vigentes, leida entre backticks.
# Leer toda la fila parece mas tolerante y es peor: la columna «Motivo» tiene prosa con
# backticks, y un `.github/README.md` mencionado al pasar apaga el hallazgo justo donde
# tendria que sonar. (Lo verifico el test negativo de este script.)
declared_files=$(
  awk '
    /^## / { in_s = (index(tolower($0), "parches vigentes") > 0); col = 0; hdr = 0; next }
    !in_s || !/^[[:space:]]*\|/ { next }
    {
      line = $0
      sub(/^[[:space:]]*\|/, "", line); sub(/\|[[:space:]]*$/, "", line)
      n = split(line, c, "|")
      sep = 1
      for (i = 1; i <= n; i++) { t = c[i]; gsub(/[[:space:]:-]/, "", t); if (t != "") sep = 0 }
      if (sep) next                                   # fila separadora |---|---|
      if (!hdr) {                                     # primera fila = encabezado
        hdr = 1
        for (i = 1; i <= n; i++) if (index(tolower(c[i]), "archivo")) col = i
        next
      }
      if (col == 0 || col > n) next
      cell = c[col]
      while (match(cell, /`[^`]+`/)) {
        tok = substr(cell, RSTART + 1, RLENGTH - 2)
        if (tok ~ /\// || tok ~ /\.(go|py|ya?ml|md|json|tsx?|sh)$/) print tok
        cell = substr(cell, RSTART + RLENGTH)
      }
    }
  ' "$PATCHES" | sed 's#^/##' | sort -u
)
[ -n "$declared_files" ] || say "  nota: no lei ninguna ruta de la columna «Archivos» de $PATCHES"
changed=$(git diff --name-only "$MIRROR_SHA" "$MAIN_SHA")
if [ -z "$changed" ]; then
  ok "sin diff contra $MIRROR_REF"
else
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    covered=0
    for o in $OWNED; do
      case "$o" in */) [ "${f#"$o"}" != "$f" ] && covered=1 ;; *) [ "$f" = "$o" ] && covered=1 ;; esac
    done
    while IFS= read -r d; do
      [ -n "$d" ] || continue
      # Cobertura exacta, o por prefijo de directorio si la fila declara un directorio.
      case "$d" in */) [ "${f#"$d"}" != "$f" ] && covered=1 ;; esac
      [ "$f" = "$d" ] && covered=1
    done <<< "$declared_files"
    if [ "$covered" = 1 ]; then ok "$f"; else bad "$f difiere del upstream y no tiene fila en $PATCHES"; fi
  done <<< "$changed"
fi

say "== 5. El pin contiene lo que justifica el pin"
if [ -z "$REQUIRED_AT_PIN" ] && [ -z "$REQUIRED_CONTENT_FILE" ]; then
  skip "este espejo no declara contenido requerido en el pin"
else
  for p in $REQUIRED_AT_PIN; do
    if git cat-file -e "$MIRROR_SHA:$p" 2>/dev/null; then ok "existe $p"; else bad "falta $p en $MIRROR_REF ($MIRROR_SHA)"; fi
  done
  if [ -n "$REQUIRED_CONTENT_FILE" ]; then
    if git show "$MIRROR_SHA:$REQUIRED_CONTENT_FILE" 2>/dev/null | grep -q -- "$REQUIRED_CONTENT_NEEDLE"; then
      ok "$REQUIRED_CONTENT_FILE declara '$REQUIRED_CONTENT_NEEDLE'"
    else
      bad "$REQUIRED_CONTENT_FILE no declara '$REQUIRED_CONTENT_NEEDLE' en $MIRROR_REF"
    fi
  fi
fi

say "== 6. El ClusterRole del cluster-agent conserva su forma"
# Por que este check vive aca y no solo en verify-cluster-agent-rbac.sh: ese script
# necesita helm + yq y NO lo corre nadie automaticamente — Actions esta deshabilitado
# a nivel repo en los tres espejos, asi que ningun workflow se dispara en un PR. Este
# es bash + git y corre en el mismo comando que ya es convencion correr en cada PR.
#
# Los dos modos de falla que reintroduce un rebase descuidado:
#   a) los permisos de workload vuelven a un Role namespaced -> no se despliega nada;
#   b) RBAC vuelve con verbos de escritura -> vuelve la escalacion que cerro T12.
for cr in $AGENT_CLUSTERROLES; do
  if ! git cat-file -e "$MAIN_SHA:$cr" 2>/dev/null; then
    bad "falta $cr en $MAIN_REF"
    continue
  fi
  check_agent_clusterrole "$cr" "$(git show "$MAIN_SHA:$cr")"
done

say ""
if [ "$fail" = 0 ]; then say "verify-fork: VERDE"; else say "verify-fork: ROJO"; fi
exit "$fail"
