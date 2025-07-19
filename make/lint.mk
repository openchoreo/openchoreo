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

.PHONY: license-check
license-check: ## Check all Go files for license headers
	@CHECK_ONLY=1 $(LICENSE_TOOL) -check-only -c $(LICENSE_HOLDER) $(ALL_GO_FILES)

.PHONY: license-fix
license-fix: ## Add license headers to all Go files
	@$(LICENSE_TOOL) -c $(LICENSE_HOLDER) $(ALL_GO_FILES)

# Binary file extensions to exclude from newline checks
BINARY_EXTENSIONS := png jpg jpeg gif ico pdf zip tar gz tgz bin exe so dylib dll woff woff2 ttf eot jar war

# Create regex pattern for binary files
BINARY_PATTERN := $(shell echo $(BINARY_EXTENSIONS) | sed 's/ /|/g')

.PHONY: newline-check
newline-check: ## Check for missing trailing newlines in all Git-tracked text files
	@echo "Checking all Git-tracked text files for missing trailing newlines..."
	@files_without_newline=$$(git ls-files | grep -v -E '\.($(BINARY_PATTERN))$$' | while read file; do \
		if [ -f "$$file" ] && [ -s "$$file" ] && [ "$$(tail -c1 "$$file" 2>/dev/null)" != "" ]; then \
			echo "$$file"; \
		fi; \
	done); \
	if [ -n "$$files_without_newline" ]; then \
		echo "Files missing trailing newlines:"; \
		echo "$$files_without_newline"; \
		echo "Run 'make newline-fix' to fix these files"; \
		exit 1; \
	else \
		echo "✓ All Git-tracked text files have trailing newlines"; \
	fi

.PHONY: newline-fix
newline-fix: ## Add missing trailing newlines to all Git-tracked text files
	@echo "Adding trailing newlines to all Git-tracked text files that need them..."
	@count=0; \
	git ls-files | grep -v -E '\.($(BINARY_PATTERN))$$' | while read file; do \
		if [ -f "$$file" ] && [ -s "$$file" ] && [ "$$(tail -c1 "$$file" 2>/dev/null)" != "" ]; then \
			echo "" >> "$$file"; \
			echo "Fixed: $$file"; \
			count=$$((count + 1)); \
		fi; \
	done | tee /tmp/newline-fix-output.txt; \
	fixed_count=$$(grep -c "^Fixed:" /tmp/newline-fix-output.txt 2>/dev/null || echo 0); \
	rm -f /tmp/newline-fix-output.txt; \
	echo "✓ Fixed $$fixed_count files"

.PHONY: golangci-lint-check
golangci-lint-check: golangci-lint ## Check code with golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: golangci-lint-fix
golangci-lint-fix: golangci-lint ## Run golangci-lint with fix option
	$(GOLANGCI_LINT) run --fix

.PHONY: lint
lint: golangci-lint-check license-check newline-check ## Run golangci-lint linter, licenser, and newline check

.PHONY: lint-fix
lint-fix: golangci-lint-fix license-fix newline-fix ## Run golangci-lint linter, licenser, and newline fix to perform fixes

.PHONY: code.gen
code.gen: manifests generate go.mod.lint helm-generate ## Generate code and fix the code with linter

.PHONY: code.gen-check
code.gen-check: code.gen ## Verify the clean Git status after code generation
	@if [ ! -z "$$(git status --porcelain)" ]; then \
	  git status --porcelain; \
      echo "There are new changes after the code generation. Please run 'make code.gen' and commit the changes"; \
      exit 1; \
    fi
