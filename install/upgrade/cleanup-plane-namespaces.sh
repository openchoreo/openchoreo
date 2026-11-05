#!/usr/bin/env bash
# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

# Cleans up the per-namespace plane namespaces created before they carried
# labels: openchoreo-kv-<namespace> (Secret API secrets) and
# workflows-<namespace> (workflow runs and git secrets).
#
# For each such namespace in a plane cluster:
#   - <namespace> is gone or terminating in the control plane: the plane
#     namespace is deleted. external-secrets then removes its pushed values
#     from the secret store.
#   - <namespace> exists: the plane namespace gets the labels the plane
#     controllers use to delete it when <namespace> is deleted.
#
# In both cases, PushSecrets created without store cleanup, such as git
# secrets pushed by older releases, get it turned on first, so deleting them
# also removes their values from the store. A plane namespace that is already
# being deleted is skipped, as its PushSecrets can no longer get store cleanup.
#
# Any failed lookup stops the script before it changes the namespace involved.
#
# Prints the planned actions and changes nothing unless --apply is given.
#
# Usage:
#   ./cleanup-plane-namespaces.sh --cp-context CTX --plane-context CTX [--plane-context CTX ...] [--skip NAME ...] [--apply]
#
# Options:
#   --cp-context     kubectl context of the control plane cluster
#   --plane-context  kubectl context of a data, workflow or observability plane
#                    cluster; repeat for each plane cluster. Use the control
#                    plane context again for a single-cluster install.
#   --skip           name of a plane namespace to leave untouched, such as an
#                    unrelated namespace that follows the same naming pattern;
#                    repeat for each
#   --apply          perform the actions instead of printing them

set -euo pipefail

LABEL_NAMESPACE="openchoreo.dev/namespace"
# Each entry is "<name prefix> <marker label>".
KINDS=(
    "openchoreo-kv- openchoreo.dev/kv-namespace"
    "workflows- openchoreo.dev/workflow-namespace"
)

CP_CONTEXT=""
PLANE_CONTEXTS=()
SKIP=()
APPLY=false

usage() {
    sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \{0,1\}//'
    exit 1
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --cp-context)
            CP_CONTEXT="${2:-}"
            shift 2
            ;;
        --plane-context)
            # An empty value would make kubectl use the current context.
            [[ -n "${2:-}" ]] || usage
            PLANE_CONTEXTS+=("$2")
            shift 2
            ;;
        --skip)
            SKIP+=("${2:-}")
            shift 2
            ;;
        --apply)
            APPLY=true
            shift
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            ;;
    esac
done

if [[ -z "$CP_CONTEXT" || ${#PLANE_CONTEXTS[@]} -eq 0 ]]; then
    usage
fi

is_skipped() {
    local s
    for s in ${SKIP[@]+"${SKIP[@]}"}; do
        [[ "$s" == "$1" ]] && return 0
    done
    return 1
}

# cp_namespace_state prints "live", or "gone" when the control plane namespace
# is absent or terminating. It fails when the namespace can't be looked up.
cp_namespace_state() {
    local name="$1" found deleting
    found=$(kubectl --context "$CP_CONTEXT" get namespace "$name" --ignore-not-found -o name) || return 1
    if [[ -z "$found" ]]; then
        echo gone
        return
    fi
    deleting=$(kubectl --context "$CP_CONTEXT" get namespace "$name" -o jsonpath='{.metadata.deletionTimestamp}') || return 1
    if [[ -n "$deleting" ]]; then
        echo gone
    else
        echo live
    fi
}

run() {
    if [[ "$APPLY" == true ]]; then
        "$@"
    fi
}

# enable_store_cleanup turns on store cleanup for the PushSecrets in a namespace
# that were created without it, and waits until external-secrets has taken over
# the cleanup of every PushSecret in it.
# A failed lookup or a wait that times out stops the script, so a namespace is
# never deleted or labeled with its values left without cleanup.
enable_store_cleanup() {
    local plane="$1" ns="$2" list err ps policy
    err=$(mktemp)
    if ! list=$(kubectl --context "$plane" -n "$ns" get pushsecrets 2>"$err" \
        -o jsonpath='{range .items[*]}{.metadata.name}{"|"}{.spec.deletionPolicy}{"\n"}{end}'); then
        # A plane without external-secrets has no PushSecrets.
        if grep -q "doesn't have a resource type" "$err"; then
            rm -f "$err"
            return 0
        fi
        local msg
        msg=$(cat "$err")
        rm -f "$err"
        fail "listing PushSecrets in $ns on $plane: $msg"
    fi
    rm -f "$err"
    while IFS="|" read -r ps policy; do
        [[ -n "$ps" ]] || continue
        if [[ "$policy" != "Delete" ]]; then
            echo "          enable store cleanup on PushSecret $ps"
            run kubectl --context "$plane" -n "$ns" patch pushsecret "$ps" --type=merge \
                -p '{"spec":{"deletionPolicy":"Delete"}}'
        fi
        # Also covers a PushSecret patched by an earlier run whose wait timed out.
        run kubectl --context "$plane" -n "$ns" wait pushsecret "$ps" --timeout=60s \
            --for=jsonpath='{.metadata.finalizers[0]}'=pushsecret.externalsecrets.io/finalizer
    done <<<"$list"
}

mode="dry run"
[[ "$APPLY" == true ]] && mode="apply"
echo "Mode: $mode"

for plane in "${PLANE_CONTEXTS[@]}"; do
    echo "Plane context: $plane"
    for kind in "${KINDS[@]}"; do
        read -r prefix marker <<<"$kind"
        marker_path="${marker//./\\.}"
        listing=$(kubectl --context "$plane" get namespaces -o \
            jsonpath="{range .items[*]}{.metadata.name}{\"|\"}{.metadata.deletionTimestamp}{\"|\"}{.metadata.labels.${marker_path}}{\"|\"}{.metadata.labels.openchoreo\\.dev/namespace}{\"\\n\"}{end}") ||
            fail "listing namespaces on $plane"
        while IFS="|" read -r name deleting marker_label ns_label; do
            [[ "$name" == "$prefix"* && "$name" != "$prefix" ]] || continue
            if is_skipped "$name"; then
                echo "  skip    $name (--skip)"
                continue
            fi
            if [[ -n "$deleting" ]]; then
                echo "  skip    $name (already terminating)"
                continue
            fi
            cp_ns="${name#"$prefix"}"
            state=$(cp_namespace_state "$cp_ns") || fail "looking up namespace $cp_ns in $CP_CONTEXT"
            case $state in
                gone)
                    echo "  delete  $name (namespace $cp_ns is gone)"
                    enable_store_cleanup "$plane" "$name"
                    run kubectl --context "$plane" delete namespace "$name" --wait=false
                    ;;
                live)
                    if [[ "$marker_label" == "true" && "$ns_label" == "$cp_ns" ]]; then
                        echo "  keep    $name (labeled)"
                        enable_store_cleanup "$plane" "$name"
                    else
                        echo "  label   $name (namespace $cp_ns exists)"
                        enable_store_cleanup "$plane" "$name"
                        run kubectl --context "$plane" label namespace "$name" --overwrite \
                            "$marker=true" "$LABEL_NAMESPACE=$cp_ns"
                    fi
                    ;;
            esac
        done <<<"$listing"
    done
done

if [[ "$APPLY" != true ]]; then
    echo "Nothing changed. Re-run with --apply to perform these actions."
fi
