#!/usr/bin/env bash
# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

# Pins the observability community-module chart versions used by the install
# scripts, e2e Makefile and multi-cluster README. main tracks 0.0.0-latest-dev;
# the release orchestrator pins released versions when it cuts a release branch.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULES_REPO="oci://ghcr.io/openchoreo/helm-charts"

# chart|flag|shell var|make var ("-" = not tracked there)
MODULES="observability-logs-opensearch|--logs-opensearch-version|LOGS_OPENSEARCH_VERSION|OBSERVABILITY_LOGS_OPENSEARCH_VERSION
observability-tracing-opensearch|--tracing-opensearch-version|TRACES_OPENSEARCH_VERSION|OBSERVABILITY_TRACES_OPENSEARCH_VERSION
observability-metrics-prometheus|--metrics-prometheus-version|METRICS_PROMETHEUS_VERSION|OBSERVABILITY_METRICS_PROMETHEUS_VERSION
observability-events-otel-collector|--events-otel-collector-version|EVENTS_OTEL_COLLECTOR_VERSION|-
observability-logs-openobserve|--logs-openobserve-version|-|OBSERVABILITY_LOGS_OPENOBSERVE_VERSION"

# The README declares the shell vars as `export VAR=...` for its commands to use.
SHELL_FILES="install/k3d/k3d-install.sh install/k3d/k3d-observability-plane.sh install/quick-start/.config.sh install/k3d/multi-cluster/README.md"
MAKE_FILE="make/e2e.mk"
TRACKED_FILES="$SHELL_FILES $MAKE_FILE"

SEMVER_RE='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'

# mode=extract prints "<line>\t<chart>\t<version>" per tracked location;
# mode=rewrite prints the file with versions from `pins` substituted.
# `map` is "<var>=<chart>;..." for the shell (VAR="x", VAR=x, export VAR=x)
# or make (VAR ?= x) variables tracked in the file.
# shellcheck disable=SC2016
AWK_PROG='
function track(chart, val) {
    if (mode == "extract") printf "%d\t%s\t%s\n", NR, chart, val
}
BEGIN {
    n = split(map, entries, ";")
    for (i = 1; i <= n; i++) {
        eq = index(entries[i], "=")
        if (eq == 0) continue
        nk++
        key[nk] = substr(entries[i], 1, eq - 1)
        chartOf[key[nk]] = substr(entries[i], eq + 1)
    }
    n = split(pins, entries, ";")
    for (i = 1; i <= n; i++) {
        eq = index(entries[i], "=")
        if (eq == 0) continue
        pin[substr(entries[i], 1, eq - 1)] = substr(entries[i], eq + 1)
    }
}
kind == "shell" {
    for (i = 1; i <= nk; i++) {
        k = key[i]
        if (index($0, k "=") == 1) prefix = k "="
        else if (index($0, "export " k "=") == 1) prefix = "export " k "="
        else continue
        rest = substr($0, length(prefix) + 1)
        c = chartOf[k]
        if (substr(rest, 1, 1) == "\"") {
            q = index(substr(rest, 2), "\"")
            if (q == 0) break
            track(c, substr(rest, 2, q - 1))
            if (c in pin) $0 = prefix "\"" pin[c] "\"" substr(rest, q + 2)
        } else {
            match(rest, /^[^ \t#;]*/)
            track(c, substr(rest, 1, RLENGTH))
            if (c in pin) $0 = prefix pin[c] substr(rest, RLENGTH + 1)
        }
        break
    }
}
kind == "make" {
    for (i = 1; i <= nk; i++) {
        k = key[i]
        if (index($0, k) != 1) continue
        rest = substr($0, length(k) + 1)
        if (!match(rest, /^[ \t]*[?]=[ \t]*/)) continue
        prefix = substr($0, 1, length(k) + RLENGTH)
        val = substr(rest, RLENGTH + 1)
        sub(/[ \t]+$/, "", val)
        c = chartOf[k]
        track(c, val)
        if (c in pin) $0 = prefix pin[c]
        break
    }
}
mode == "rewrite" { print }
'

err() { echo "ERROR: $*" >&2; }
note() { echo "NOTE: $*" >&2; }
die() { err "$*"; exit 1; }

module_charts() {
    local chart _
    while IFS='|' read -r chart _; do
        echo "$chart"
    done <<< "$MODULES"
}

chart_for_flag() {
    local chart flag _
    while IFS='|' read -r chart flag _; do
        if [[ "$flag" == "$1" ]]; then
            echo "$chart"
            return 0
        fi
    done <<< "$MODULES"
    return 1
}

flag_for_chart() {
    local chart flag _
    while IFS='|' read -r chart flag _; do
        if [[ "$chart" == "$1" ]]; then
            echo "$flag"
            return 0
        fi
    done <<< "$MODULES"
    return 1
}

usage() {
    local chart flag _
    cat <<EOF
Usage:
  $(basename "$0") --<module>-version <version> [--<module>-version <version>...]
      Pin the given modules in every tracked file.
  $(basename "$0") --validate [--remote] --<module>-version <version>...
      Require a released version for every module. --remote also checks that
      each chart version is published to ${MODULES_REPO}.
  $(basename "$0") --check [--ref <rev>]
      Fail if any tracked location is unpinned (0.0.0-* or latest-dev).
      --ref reads the files at a git revision instead of the working tree.

Modules:
$(while IFS='|' read -r chart flag _; do printf '  %-34s %s\n' "$flag" "$chart"; done <<< "$MODULES")

Tracked files:
$(printf '%s' "$TRACKED_FILES" | tr ' ' '\n' | sed 's/^/  /')
EOF
}

kind_of() {
    case "$1" in
        *.mk) echo make ;;
        *) echo shell ;;
    esac
}

# Prints the awk key map ("<var>=<chart>;...") for a tracked file.
map_for() {
    local kind chart shell_var make_var out=""
    kind="$(kind_of "$1")"
    while IFS='|' read -r chart _ shell_var make_var; do
        case "$kind" in
            shell) if [[ "$shell_var" != "-" ]]; then out+="${shell_var}=${chart};"; fi ;;
            make) if [[ "$make_var" != "-" ]]; then out+="${make_var}=${chart};"; fi ;;
        esac
    done <<< "$MODULES"
    printf '%s' "$out"
}

# Prints the charts a tracked file is expected to pin.
charts_for() {
    local entry
    local IFS=';'
    for entry in $(map_for "$1"); do
        echo "${entry#*=}"
    done
}

is_unpinned() {
    [[ -z "$1" || "$1" == *0.0.0-* || "$1" == *latest-dev* ]]
}

read_file() {
    if [[ -n "$REF" ]]; then
        git -C "$ROOT" show "${REF}:$1" 2>/dev/null
    else
        cat "$ROOT/$1" 2>/dev/null
    fi
}

# Prints "<line>\t<chart>\t<version>" for each tracked location in a file.
locations() {
    local content
    content="$(read_file "$1")" || return 0
    printf '%s\n' "$content" |
        awk -v mode=extract -v kind="$(kind_of "$1")" -v map="$(map_for "$1")" -v pins="" "$AWK_PROG"
}

# Validates the --<module>-version arguments (ARGS) into PINS, one
# "<chart>=<version>" per line. An empty version counts as not given.
parse_pins() {
    local require_all="$1" chart version flag errors=0 seen=""
    PINS=""
    while IFS=$'\t' read -r chart version; do
        if [[ -z "$chart" || -z "$version" ]]; then
            continue
        fi
        flag="$(flag_for_chart "$chart")"
        if printf '%s\n' "$seen" | grep -qx "$chart"; then
            err "${flag} is given more than once"
            errors=1
            continue
        fi
        seen+="${chart}"$'\n'
        if is_unpinned "$version" || ! [[ "$version" =~ $SEMVER_RE ]]; then
            err "invalid ${flag} '${version}' (expected a released X.Y.Z version)"
            errors=1
        else
            PINS+="${chart}=${version}"$'\n'
        fi
    done <<< "$ARGS"
    if [[ "$require_all" == "true" ]]; then
        for chart in $(module_charts); do
            if ! printf '%s\n' "$seen" | grep -qx "$chart"; then
                err "missing $(flag_for_chart "$chart")"
                errors=1
            fi
        done
    elif [[ -z "$seen" && "$errors" -eq 0 ]]; then
        usage >&2
        die "no module versions given"
    fi
    if [[ "$errors" -ne 0 ]]; then
        exit 1
    fi
    PINS="${PINS%$'\n'}"
}

validate() {
    local chart version status=0
    parse_pins true
    if [[ "$REMOTE" == "true" ]]; then
        command -v helm >/dev/null 2>&1 || die "helm is required for --remote"
        while IFS='=' read -r chart version; do
            if helm show chart "${MODULES_REPO}/${chart}" --version "$version" >/dev/null 2>&1; then
                echo "found ${MODULES_REPO}/${chart}:${version}"
            else
                err "${MODULES_REPO}/${chart}:${version} is not published"
                status=1
            fi
        done <<< "$PINS"
    fi
    return "$status"
}

apply() {
    local f tmp chart version found mismatched line status=0
    parse_pins false
    tmp="$(mktemp)"
    # shellcheck disable=SC2064
    trap "rm -f '$tmp'" EXIT
    for f in $TRACKED_FILES; do
        [[ -f "$ROOT/$f" ]] || die "$f not found"
        awk -v mode=rewrite -v kind="$(kind_of "$f")" -v map="$(map_for "$f")" \
            -v pins="$(printf '%s' "$PINS" | tr '\n' ';')" "$AWK_PROG" "$ROOT/$f" > "$tmp"
        if ! cmp -s "$tmp" "$ROOT/$f"; then
            # Write in place to keep file modes.
            cat "$tmp" > "$ROOT/$f"
            echo "updated $f"
        fi
    done
    # Every expected location must now carry the requested version, so a
    # refactor of a tracked file cannot silently turn pinning into a no-op.
    while IFS='=' read -r chart version; do
        for f in $TRACKED_FILES; do
            if ! charts_for "$f" | grep -qx "$chart"; then
                continue
            fi
            found="$(locations "$f" | awk -F'\t' -v c="$chart" '$2 == c')"
            if [[ -z "$found" ]]; then
                err "$f: no version location found for ${chart}"
                status=1
                continue
            fi
            mismatched="$(printf '%s\n' "$found" | awk -F'\t' -v v="$version" '$3 != v { print $1 }')"
            for line in $mismatched; do
                err "$f:$line: ${chart} was not pinned to ${version}"
                status=1
            done
        done
        echo "pinned ${chart} ${version}"
    done <<< "$PINS"
    return "$status"
}

check() {
    local f locs chart line version seen="" drift status=0
    if [[ -n "$REF" ]]; then
        git -C "$ROOT" rev-parse --verify --quiet "${REF}^{commit}" >/dev/null || die "unknown revision '${REF}'"
    fi
    for f in $TRACKED_FILES; do
        if ! read_file "$f" >/dev/null; then
            note "$f not found${REF:+ at ${REF}}; skipped"
            continue
        fi
        locs="$(locations "$f")"
        # Missing locations are notes, not failures: older release branches
        # predate some of the tracked modules.
        for chart in $(charts_for "$f"); do
            if ! printf '%s\n' "$locs" | cut -f2 | grep -qx "$chart"; then
                note "$f: no version found for ${chart}"
            fi
        done
        while IFS=$'\t' read -r line chart version; do
            if [[ -z "$line" ]]; then
                continue
            fi
            if is_unpinned "$version"; then
                err "$f:$line: ${chart} is not pinned (${version:-empty})"
                status=1
            fi
            seen+="${chart}"$'\t'"${version}"$'\n'
        done <<< "$locs"
    done
    drift="$(printf '%s' "$seen" | sort -u | cut -f1 | uniq -d)"
    for chart in $drift; do
        note "${chart} has differing versions: $(printf '%s' "$seen" | awk -F'\t' -v c="$chart" '$1 == c { print $2 }' | sort -u | tr '\n' ' ')"
    done
    if [[ "$status" -eq 0 ]]; then
        echo "observability module versions are pinned${REF:+ at ${REF}}:"
        printf '%s' "$seen" | sort -u | sed 's/^/  /'
    fi
    return "$status"
}

MODE="apply"
REMOTE="false"
REF=""
# "<chart>\t<version>" per --<module>-version argument, in the order given.
ARGS=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --check) MODE="check"; shift ;;
        --validate) MODE="validate"; shift ;;
        --remote) REMOTE="true"; shift ;;
        --ref)
            [[ $# -ge 2 ]] || die "--ref requires a revision"
            REF="$2"
            shift 2
            ;;
        -h|--help) usage; exit 0 ;;
        -*)
            if ! chart="$(chart_for_flag "$1")"; then
                usage >&2
                die "unknown option: $1"
            fi
            [[ $# -ge 2 ]] || die "$1 requires a version"
            ARGS+="${chart}"$'\t'"$2"$'\n'
            shift 2
            ;;
        *) usage >&2; die "unexpected argument: $1" ;;
    esac
done

if [[ -n "$REF" && "$MODE" != "check" ]]; then
    die "--ref is only valid with --check"
fi
if [[ "$REMOTE" == "true" && "$MODE" != "validate" ]]; then
    die "--remote is only valid with --validate"
fi

case "$MODE" in
    check)
        [[ -z "$ARGS" ]] || die "--check takes no module versions"
        check
        ;;
    validate) validate ;;
    apply) apply ;;
esac
