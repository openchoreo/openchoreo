#!/bin/bash

# Release script for Backstage OpenChoreo
# This script helps create consistent releases for both Docker image and Helm chart

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CHART_PATH="${ROOT_DIR}/charts/backstage-openchoreo"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_usage() {
    echo "Usage: $0 [OPTIONS] VERSION"
    echo ""
    echo "Creates a new release with the specified version."
    echo ""
    echo "OPTIONS:"
    echo "  -h, --help     Show this help message"
    echo "  -d, --dry-run  Show what would be done without making changes"
    echo ""
    echo "VERSION:"
    echo "  Semantic version (e.g., 1.0.0, 1.2.3-beta.1)"
    echo ""
    echo "Examples:"
    echo "  $0 1.0.0"
    echo "  $0 1.2.3-beta.1"
    echo "  $0 --dry-run 2.0.0"
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

validate_version() {
    local version=$1
    # Basic semantic version validation
    if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
        log_error "Invalid version format: $version"
        log_error "Version should follow semantic versioning (e.g., 1.0.0, 1.2.3-beta.1)"
        exit 1
    fi
}

check_git_status() {
    if [[ -n $(git status --porcelain) ]]; then
        log_error "Git working directory is not clean. Please commit or stash changes."
        exit 1
    fi
}

check_main_branch() {
    local current_branch=$(git branch --show-current)
    if [[ "$current_branch" != "main" ]]; then
        log_warn "You are not on the main branch (current: $current_branch)"
        read -p "Continue? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

update_chart_version() {
    local version=$1
    local dry_run=$2
    
    log_info "Updating Chart.yaml version to $version"
    
    if [[ "$dry_run" == "true" ]]; then
        log_info "[DRY RUN] Would update version in $CHART_PATH/Chart.yaml"
        log_info "[DRY RUN] Would update appVersion in $CHART_PATH/Chart.yaml"
        log_info "[DRY RUN] Would update image tag in $CHART_PATH/values.yaml"
    else
        # Update Chart.yaml
        sed -i.bak "s/^version:.*/version: $version/" "$CHART_PATH/Chart.yaml"
        sed -i.bak "s/^appVersion:.*/appVersion: \"$version\"/" "$CHART_PATH/Chart.yaml"
        
        # Update values.yaml image tag
        sed -i.bak "s/tag: \".*\"/tag: \"v$version\"/" "$CHART_PATH/values.yaml"
        
        # Remove backup files
        rm -f "$CHART_PATH/Chart.yaml.bak" "$CHART_PATH/values.yaml.bak"
    fi
}

create_git_tag() {
    local version=$1
    local dry_run=$2
    local tag="v$version"
    
    log_info "Creating git tag $tag"
    
    if [[ "$dry_run" == "true" ]]; then
        log_info "[DRY RUN] Would create git tag: $tag"
        log_info "[DRY RUN] Would push tag to origin"
    else
        git add "$CHART_PATH/Chart.yaml" "$CHART_PATH/values.yaml"
        git commit -m "Release version $version

- Update chart version to $version
- Update appVersion to $version  
- Update image tag to v$version"
        
        git tag -a "$tag" -m "Release $version"
        
        log_info "Tag $tag created locally"
        log_warn "Remember to push the tag: git push origin $tag"
        log_warn "This will trigger both Docker build and Helm chart release workflows"
    fi
}

main() {
    local dry_run=false
    local version=""
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                print_usage
                exit 0
                ;;
            -d|--dry-run)
                dry_run=true
                shift
                ;;
            -*)
                log_error "Unknown option: $1"
                print_usage
                exit 1
                ;;
            *)
                if [[ -z "$version" ]]; then
                    version=$1
                else
                    log_error "Multiple versions specified"
                    print_usage
                    exit 1
                fi
                shift
                ;;
        esac
    done
    
    # Validate arguments
    if [[ -z "$version" ]]; then
        log_error "Version is required"
        print_usage
        exit 1
    fi
    
    validate_version "$version"
    
    if [[ "$dry_run" == "true" ]]; then
        log_info "Running in dry-run mode - no changes will be made"
    fi
    
    # Pre-flight checks
    check_git_status
    check_main_branch
    
    # Check if tag already exists
    if git tag -l | grep -q "^v$version$"; then
        log_error "Tag v$version already exists"
        exit 1
    fi
    
    log_info "Creating release for version $version"
    
    # Update versions
    update_chart_version "$version" "$dry_run"
    
    # Create git tag
    create_git_tag "$version" "$dry_run"
    
    if [[ "$dry_run" != "true" ]]; then
        log_info "Release $version prepared successfully!"
        log_info "Next steps:"
        log_info "1. Review the changes: git show HEAD"
        log_info "2. Push the tag: git push origin v$version"
        log_info "3. Monitor GitHub Actions for Docker and Helm releases"
    fi
}

main "$@"