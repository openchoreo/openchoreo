# This makefile contains all the make targets related code generation and linting.

##@ Code Generation and Linting


.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: code.gen
code.gen: manifests generate lint-fix helm-generate ## Generate code and fix the code with linter

.PHONY: code.gen-check
code.gen-check: code.gen ## Verify the clean Git status after code generation
	@if [ ! -z "$$(git status --porcelain)" ]; then \
	  git status --porcelain; \
      echo "There are new changes after the code generation. Please run 'make code.gen' and commit the changes"; \
      exit 1; \
    fi
