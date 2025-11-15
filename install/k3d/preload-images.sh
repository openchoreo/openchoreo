#!/usr/bin/env bash
set -eo pipefail

# Script to preload Docker images into k3d cluster
# This improves deployment speed by pulling images on host then importing to k3d
# instead of pulling from within the cluster

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_DIR="${SCRIPT_DIR}/../helm"

# Default values
CLUSTER_NAME=""
INCLUDE_CONTROL_PLANE=false
INCLUDE_DATA_PLANE=false
INCLUDE_BUILD_PLANE=false
INCLUDE_OBSERVABILITY_PLANE=false
CP_VALUES=""
DP_VALUES=""
BP_VALUES=""
OP_VALUES=""
OPENCHOREO_CHART_VERSION=""
PARALLEL_PULLS=4
HELM_REPO="oci://ghcr.io/openchoreo/helm-charts"
USE_LOCAL_CHARTS=false
CP_CHART=""
DP_CHART=""
BP_CHART=""
OP_CHART=""
EXTRA_IMAGES=()

# Color codes for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
RESET='\033[0m'

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${RESET} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${RESET} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${RESET} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${RESET} $*"
}

# Usage function
usage() {
    cat <<EOF
Usage: $0 --cluster CLUSTER_NAME [OPTIONS]

Preload Docker images into a k3d cluster by pulling on host and importing.

Required:
  --cluster NAME              k3d cluster name

Plane Selection (at least one required):
  --control-plane             Include Control Plane images
  --data-plane                Include Data Plane images
  --build-plane               Include Build Plane images
  --observability-plane       Include Observability Plane images

Optional:
  --cp-values FILE            Helm values file for Control Plane
  --dp-values FILE            Helm values file for Data Plane
  --bp-values FILE            Helm values file for Build Plane
  --op-values FILE            Helm values file for Observability Plane
  --version VERSION           Helm chart version for OCI registry (default: empty, pulls latest)
                              Only used when --local-charts is NOT specified
  --parallel N                Number of parallel docker pulls (default: 4)
  --helm-repo URL             OCI Helm repository URL (default: oci://ghcr.io/openchoreo/helm-charts)
  --local-charts              Use local chart paths instead of OCI registry
  --cp-chart PATH/URL         Custom Control Plane chart path or OCI URL
  --dp-chart PATH/URL         Custom Data Plane chart path or OCI URL
  --bp-chart PATH/URL         Custom Build Plane chart path or OCI URL
  --op-chart PATH/URL         Custom Observability Plane chart path or OCI URL
  --extra-images IMAGES       Comma-separated list of additional images to preload
  --help                      Show this help message

Examples:
  # Local development with local charts
  $0 --cluster openchoreo-dev --local-charts --control-plane --data-plane

  # Using OCI registry charts with specific version
  $0 --cluster openchoreo-prod --control-plane --data-plane --version 0.1.0

  # Using OCI registry charts (pulls latest)
  $0 --cluster openchoreo-prod --control-plane --data-plane

  # Quick-start with local charts and custom values
  $0 --cluster openchoreo-quick-start --local-charts \\
    --control-plane --cp-values install/quick-start/.values-cp.yaml \\
    --data-plane --dp-values install/quick-start/.values-dp.yaml

  # Mix of OCI and custom chart paths
  $0 --cluster openchoreo \\
    --control-plane --cp-chart oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \\
    --data-plane --dp-chart /path/to/custom/data-plane

  # With extra images
  $0 --cluster openchoreo-dev --local-charts --control-plane \\
    --extra-images "curlimages/curl:8.4.0,envoyproxy/envoy:distroless-v1.35.6"
EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --cluster)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --control-plane)
            INCLUDE_CONTROL_PLANE=true
            shift
            ;;
        --data-plane)
            INCLUDE_DATA_PLANE=true
            shift
            ;;
        --build-plane)
            INCLUDE_BUILD_PLANE=true
            shift
            ;;
        --observability-plane)
            INCLUDE_OBSERVABILITY_PLANE=true
            shift
            ;;
        --cp-values)
            CP_VALUES="$2"
            shift 2
            ;;
        --dp-values)
            DP_VALUES="$2"
            shift 2
            ;;
        --bp-values)
            BP_VALUES="$2"
            shift 2
            ;;
        --op-values)
            OP_VALUES="$2"
            shift 2
            ;;
        --version)
            OPENCHOREO_CHART_VERSION="$2"
            shift 2
            ;;
        --parallel)
            if ! [[ "$2" =~ ^[1-9][0-9]*$ ]]; then
                log_error "Invalid --parallel value: $2 (must be positive integer)"
                exit 1
            fi
            PARALLEL_PULLS="$2"
            shift 2
            ;;
        --helm-repo)
            HELM_REPO="$2"
            shift 2
            ;;
        --local-charts)
            USE_LOCAL_CHARTS=true
            shift
            ;;
        --cp-chart)
            CP_CHART="$2"
            shift 2
            ;;
        --dp-chart)
            DP_CHART="$2"
            shift 2
            ;;
        --bp-chart)
            BP_CHART="$2"
            shift 2
            ;;
        --op-chart)
            OP_CHART="$2"
            shift 2
            ;;
        --extra-images)
            # Parse comma-separated images
            IFS=',' read -ra images_array <<< "$2"
            for img in "${images_array[@]}"; do
                # Trim whitespace
                img=$(echo "$img" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
                if [[ -n "$img" ]]; then
                    EXTRA_IMAGES+=("$img")
                fi
            done
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$CLUSTER_NAME" ]]; then
    log_error "Cluster name is required"
    usage
    exit 1
fi

# Check if at least one plane is selected
if [[ "$INCLUDE_CONTROL_PLANE" == "false" ]] && \
   [[ "$INCLUDE_DATA_PLANE" == "false" ]] && \
   [[ "$INCLUDE_BUILD_PLANE" == "false" ]] && \
   [[ "$INCLUDE_OBSERVABILITY_PLANE" == "false" ]]; then
    log_error "At least one plane must be selected"
    usage
    exit 1
fi

# Check if k3d cluster exists
if ! k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
    log_error "k3d cluster '${CLUSTER_NAME}' not found"
    log_info "Available clusters:"
    k3d cluster list 2>/dev/null || echo "  (none)"
    exit 1
fi

log_info "Cluster: ${CLUSTER_NAME}"

# Resolve chart location based on flags
# Returns either local path or OCI URL
resolve_chart_location() {
    local chart_name="$1"
    local custom_chart="$2"

    # If custom chart is specified, use it directly
    if [[ -n "$custom_chart" ]]; then
        echo "$custom_chart"
        return 0
    fi

    # Use local charts if flag is set
    if [[ "$USE_LOCAL_CHARTS" == "true" ]]; then
        echo "${HELM_DIR}/${chart_name}"
        return 0
    fi

    # Default to OCI registry
    # Omit --version flag if empty to let Helm pull the latest version automatically
    if [[ -z "$OPENCHOREO_CHART_VERSION" ]]; then
        echo "${HELM_REPO}/${chart_name}"
    else
        echo "${HELM_REPO}/${chart_name} --version ${OPENCHOREO_CHART_VERSION}"
    fi
}

# Extract images from Helm chart templates
# Filters out CEL template expressions like ${workload.containers["main"].image}
# Supports both local paths and OCI chart URLs
get_helm_chart_images() {
    local chart_ref="$1"
    local values_file="$2"
    local release_name="$3"

    # Build helm template command (works for both local and OCI charts)
    local helm_cmd="helm template ${release_name} ${chart_ref}"

    # Add values file if provided
    if [[ -n "$values_file" ]]; then
        if [[ ! -f "$values_file" ]]; then
            log_warning "Values file not found: $values_file"
        else
            helm_cmd="${helm_cmd} --values ${values_file}"
        fi
    fi

    # Extract images from rendered templates
    # Filter out CEL template expressions using grep -vE '^\$\{'
    ${helm_cmd} 2>/dev/null | \
        grep -E '^\s+image:' | \
        sed 's/.*image: *//' | \
        sed 's/"//g' | \
        grep -vE '^\$\{' | \
        sort -u || true
}

# Get K3s base images (hardcoded - these depend on k3d/k3s version)
# IMPORTANT: When updating k3s version in install/k3d/*/config.yaml files,
# update these image versions to match the new k3s version.
# To find the correct versions, create a test cluster with the new k3s version:
#   k3d cluster create test --image rancher/k3s:vX.XX.X-k3sX
#   kubectl get pods -A -o jsonpath='{range .items[*]}{.spec.containers[*].image}{"\n"}{end}' | sort -u
#
# Current versions are for k3s v1.32.9-k3s1 (as configured in install/k3d configs)
get_k3s_images() {
    cat <<EOF
docker.io/rancher/mirrored-coredns-coredns:1.12.3
docker.io/rancher/local-path-provisioner:v0.0.31
docker.io/rancher/mirrored-library-traefik:3.3.6
docker.io/rancher/klipper-helm:v0.9.8-build20250709
docker.io/rancher/klipper-lb:v0.4.13
docker.io/rancher/mirrored-metrics-server:v0.8.0
EOF
}

# Collect all images based on selected planes
collect_images() {
    local all_images=()

    # Always include K3s base images
    log_info "Collecting K3s base images..." >&2
    local k3s_images=()
    while IFS= read -r line; do
        k3s_images+=("$line")
    done < <(get_k3s_images)
    all_images+=("${k3s_images[@]}")

    # Control Plane images
    if [[ "$INCLUDE_CONTROL_PLANE" == "true" ]]; then
        log_info "Collecting Control Plane images..." >&2
        local cp_chart
        cp_chart=$(resolve_chart_location "openchoreo-control-plane" "$CP_CHART")
        local cp_images=()
        while IFS= read -r line; do
            cp_images+=("$line")
        done < <(get_helm_chart_images "$cp_chart" "${CP_VALUES}" "openchoreo-cp")
        if [[ ${#cp_images[@]} -eq 0 ]]; then
            log_warning "No images found for Control Plane (helm template may have failed)" >&2
        fi
        all_images+=("${cp_images[@]}")
    fi

    # Data Plane images
    if [[ "$INCLUDE_DATA_PLANE" == "true" ]]; then
        log_info "Collecting Data Plane images..." >&2
        local dp_chart
        dp_chart=$(resolve_chart_location "openchoreo-data-plane" "$DP_CHART")
        local dp_images=()
        while IFS= read -r line; do
            dp_images+=("$line")
        done < <(get_helm_chart_images "$dp_chart" "${DP_VALUES}" "openchoreo-dp")
        if [[ ${#dp_images[@]} -eq 0 ]]; then
            log_warning "No images found for Data Plane (helm template may have failed)" >&2
        fi
        all_images+=("${dp_images[@]}")
    fi

    # Build Plane images
    if [[ "$INCLUDE_BUILD_PLANE" == "true" ]]; then
        log_info "Collecting Build Plane images..." >&2
        local bp_chart
        bp_chart=$(resolve_chart_location "openchoreo-build-plane" "$BP_CHART")
        local bp_images=()
        while IFS= read -r line; do
            bp_images+=("$line")
        done < <(get_helm_chart_images "$bp_chart" "${BP_VALUES}" "openchoreo-bp")
        if [[ ${#bp_images[@]} -eq 0 ]]; then
            log_warning "No images found for Build Plane (helm template may have failed)" >&2
        fi
        all_images+=("${bp_images[@]}")
    fi

    # Observability Plane images
    if [[ "$INCLUDE_OBSERVABILITY_PLANE" == "true" ]]; then
        log_info "Collecting Observability Plane images..." >&2
        local op_chart
        op_chart=$(resolve_chart_location "openchoreo-observability-plane" "$OP_CHART")
        local op_images=()
        while IFS= read -r line; do
            op_images+=("$line")
        done < <(get_helm_chart_images "$op_chart" "${OP_VALUES}" "openchoreo-op")
        if [[ ${#op_images[@]} -eq 0 ]]; then
            log_warning "No images found for Observability Plane (helm template may have failed)" >&2
        fi
        all_images+=("${op_images[@]}")
    fi

    # Extra images provided by user via --extra-images flag
    if [[ ${#EXTRA_IMAGES[@]} -gt 0 ]]; then
        log_info "Adding ${#EXTRA_IMAGES[@]} extra images..." >&2
        all_images+=("${EXTRA_IMAGES[@]}")
    fi

    # Remove duplicates and output
    printf '%s\n' "${all_images[@]}" | sort -u
}

# Pull docker images with parallel execution
pull_images() {
    local images=("$@")
    local total=${#images[@]}

    log_info "Pulling ${total} Docker images (${PARALLEL_PULLS} parallel)..."

    # Function to pull a single image
    pull_single_image() {
        local image="$1"
        local index="$2"
        local total="$3"

        local stderr_output
        if stderr_output=$(docker pull "$image" 2>&1 >/dev/null); then
            log_success "[$index/$total] Pulled $image"
            return 0
        else
            log_warning "[$index/$total] Failed to pull $image"
            if [[ -n "$stderr_output" ]]; then
                echo "  ${stderr_output}" | head -2
            fi
            return 1
        fi
    }

    export -f pull_single_image
    export -f log_success
    export -f log_warning
    export GREEN YELLOW RESET

    # Pull images in parallel batches
    local index=0
    local pids=()

    for image in "${images[@]}"; do
        index=$((index + 1))
        pull_single_image "$image" "$index" "$total" &
        pids+=($!)

        # Wait for batch to complete before starting next batch
        if [[ ${#pids[@]} -ge $PARALLEL_PULLS ]]; then
            for pid in "${pids[@]}"; do
                wait "$pid" || true
            done
            pids=()
        fi
    done

    # Wait for remaining pulls
    for pid in "${pids[@]}"; do
        wait "$pid" || true
    done

    log_success "Docker pull complete"
}

# Import images to k3d cluster
import_images_to_k3d() {
    local images=("$@")
    local total=${#images[@]}
    local failed=0

    log_info "Importing ${total} images to k3d cluster '${CLUSTER_NAME}'..."

    # Import images in batches to avoid argument limit issues
    # k3d can handle multiple images, but we batch to be safe with large lists
    local batch_size=20
    local batch_count=$(( (total + batch_size - 1) / batch_size ))

    for ((i=0; i<total; i+=batch_size)); do
        local batch=("${images[@]:i:batch_size}")
        local batch_num=$(( i / batch_size + 1 ))

        if ! k3d image import "${batch[@]}" --cluster "${CLUSTER_NAME}" 2>/dev/null; then
            log_warning "Batch $batch_num/$batch_count: Some images failed to import"
            ((failed++))
        fi
    done

    if [[ $failed -gt 0 ]]; then
        log_warning "Some images may not have imported successfully"
        return 1
    else
        log_success "Successfully imported all images to cluster"
    fi
}

# Main execution
main() {
    log_info "Starting image preload for cluster '${CLUSTER_NAME}'"

    # Collect images
    local images=()
    while IFS= read -r line; do
        images+=("$line")
    done < <(collect_images)

    if [[ ${#images[@]} -eq 0 ]]; then
        log_error "No images found to preload"
        exit 1
    fi

    log_info "Found ${#images[@]} unique images to preload"

    # Pull images
    pull_images "${images[@]}"

    # Import to k3d
    import_images_to_k3d "${images[@]}"

    log_success "Image preload complete for cluster '${CLUSTER_NAME}'"
}

# Run main function
main
