# This makefile contains all the make targets related code generation and linting.

##@ Code Generation and Linting

# All project Go files, excluding generated / vendored paths
ALL_GO_FILES := $(shell \
	find . -type f -name '*.go' \
		! -path './internal/dataplane/kubernetes/types/*' \
		! -path './api/v1/zz_generated.deepcopy.go' \
	| sort)

# Path to your tool (update if different)
LICENSE_TOOL := go run ./tools/licenser/main.go
LICENSE_HOLDER := "The OpenChoreo Authors"
LICENSE_TYPE := "apache"

.PHONY: license-check
license-check:
	@CHECK_ONLY=1 $(LICENSE_TOOL) -check-only -c $(LICENSE_HOLDER) -l $(LICENSE_TYPE) $(ALL_GO_FILES)

.PHONY: license-fix
license-fix:
	@$(LICENSE_TOOL) -c $(LICENSE_HOLDER) -l $(LICENSE_TYPE) $(ALL_GO_FILES)

.PHONY: lint
lint: golangci-lint license-check ## Run golangci-lint linter and licenser
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint license-fix ## Run golangci-lint linter and licenser to perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: code.gen
code.gen: manifests generate go.mod.lint helm-generate ## Generate code and fix the code with linter

.PHONY: code.gen-check
code.gen-check: code.gen ## Verify the clean Git status after code generation
	@if [ ! -z "$$(git status --porcelain)" ]; then \
	  git status --porcelain; \
      echo "There are new changes after the code generation. Please run 'make code.gen' and commit the changes"; \
      exit 1; \
    fi
