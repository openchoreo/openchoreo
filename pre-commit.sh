#!/bin/bash

# ==============================================================================
# OpenChoreo Pre-Commit Automation Script
# ==============================================================================
# This script prepares your workspace for a commit by running:
# 1. Dependency cleanup (go mod tidy)
# 2. Kubernetes Manifest generation (CRDs)
# 3. Helm Chart generation
# 4. Code generation & Lint fixing (DeepCopy, Formatting, License headers)
# 5. Build verification
# 6. Unit Tests
# ==============================================================================

# Define colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status messages
log() {
    echo -e "${GREEN}[PRE-COMMIT]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Ensure script stops on first error
set -e

# ------------------------------------------------------------------------------
# 1. Dependency Management
# ------------------------------------------------------------------------------
log "Cleaning up Go modules (go.mod.tidy)..."
make go.mod.tidy || error "Failed to tidy go modules."

# ------------------------------------------------------------------------------
# 2. Kubernetes & Helm Generation
# ------------------------------------------------------------------------------
# It is vital to regenerate manifests if structs/APIs changed to keep CRDs in sync
log "Generating Kubernetes Manifests (CRDs, RBAC)..."
make manifests || error "Failed to generate manifests."

log "Generating Helm Charts..."
make helm-generate || error "Failed to generate helm charts."

# ------------------------------------------------------------------------------
# 3. Code Generation & Linting
# ------------------------------------------------------------------------------
# This target appears to handle DeepCopy generation AND linting/fixing
log "Running Code Generation and Linting (code.gen)..."
# Note: code.gen includes 'lint-fix', 'license-fix', and 'newline-fix' usually
make code.gen || error "Code generation or Linting failed."

# ------------------------------------------------------------------------------
# 4. Build Verification
# ------------------------------------------------------------------------------
# Ensure the code actually compiles before testing
log "Verifying build (go.build)..."
make go.build || error "Build failed. Fix compilation errors."

# ------------------------------------------------------------------------------
# 5. Testing
# ------------------------------------------------------------------------------
log "Running Unit Tests..."
make test || error "Tests failed."

# ------------------------------------------------------------------------------
# 6. Final Status Check
# ------------------------------------------------------------------------------
echo ""
log "All checks passed successfully!"
warn "The following files have been modified by the generation scripts:"
echo ""
git status -s

echo ""
log "You are ready to: git add . && git commit -m 'Your message'"
