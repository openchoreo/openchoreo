#!/bin/bash

# Release script for Backstage OpenChoreo
# This script helps create consistent releases for both Docker image and Helm chart

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CHART_PATH="${ROOT_DIR}/charts/backstage-demo"
VERSION_FILE="${ROOT_DIR}/VERSION"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Creates a new release using the version from the VERSION file."
    echo ""
    echo "OPTIONS:"
    echo "  -h, --help     Show this help message"
    echo "  -d, --dry-run  Show what would be done without making changes"
    echo ""
    echo "The version is read from the VERSION file in the repository root."
    echo "Update the VERSION file first, then run this script."
    echo ""
    echo "Examples:"
    echo "  $0"
    echo "  $0 --dry-run"
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

read_version_file() {
    if [[ ! -f "$VERSION_FILE" ]]; then
        log_error "VERSION file not found at $VERSION_FILE"
        log_error "Please create a VERSION file with the desired version number"
        exit 1
    fi
    
    local version=$(cat "$VERSION_FILE" | tr -d '\n\r')
    
    if [[ -z "$version" ]]; then
        log_error "VERSION file is empty"
        exit 1
    fi
    
    echo "$version"
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
        git tag -a "$tag" -m "Release $version"
        
        log_info "Tag $tag created locally"
        log_warn "Remember to push the tag: git push origin $tag"
        log_warn "This will trigger both Docker build and Helm chart release workflows"
    fi
}

main() {
    local dry_run=false
    
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
                log_error "Unexpected argument: $1"
                log_error "This script reads the version from the VERSION file"
                print_usage
                exit 1
                ;;
        esac
    done
    
    # Read version from file
    local version=$(read_version_file)
    
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
    
    log_info "Creating release for version $version (from VERSION file)"
    
    # Create git tag
    create_git_tag "$version" "$dry_run"
    
    if [[ "$dry_run" != "true" ]]; then
        log_info "Release $version prepared successfully!"
        log_info "Next steps:"
        log_info "1. Push the tag: git push origin v$version"
        log_info "2. Monitor GitHub Actions for Docker and Helm releases"
    fi
}

main "$@"
