# This makefile contains all the make targets related to the Python agents
# under agents/*. Each agent is a self-contained uv project; these targets
# fan the common commands out across all of them.

# Discover agents as any directory under agents/ that has a pyproject.toml.
PYTHON_AGENTS := $(sort $(notdir $(patsubst %/,%,$(dir $(wildcard agents/*/pyproject.toml)))))

##@ Python Agents

.PHONY: test
test: go.test python.test ## Run all Go and Python tests.

.PHONY: python.test
python.test: ## Run tests for all Python agents.
	@for agent in $(PYTHON_AGENTS); do \
		$(call log_info, "Testing agents/$$agent"); \
		(cd agents/$$agent && uv run --frozen --group dev pytest -q) || exit 1; \
	done

.PHONY: python.check
python.check: ## Lint all Python agents (ruff check + format check).
	@for agent in $(PYTHON_AGENTS); do \
		$(call log_info, "Checking agents/$$agent"); \
		(cd agents/$$agent && uvx ruff check . && uvx ruff format --check .) || exit 1; \
	done

.PHONY: python.fix
python.fix: ## Auto-fix lint issues in all Python agents.
	@for agent in $(PYTHON_AGENTS); do \
		(cd agents/$$agent && uvx ruff check --fix . && uvx ruff format .); \
	done

.PHONY: python.format
python.format: ## Format all Python agents.
	@for agent in $(PYTHON_AGENTS); do \
		(cd agents/$$agent && uvx ruff format .); \
	done

.PHONY: python.clean
python.clean: ## Remove Python caches across all agents.
	find agents -type d -name "__pycache__" -exec rm -rf {} +
	find agents -type d -name ".pytest_cache" -exec rm -rf {} +
	find agents -type d -name ".ruff_cache" -exec rm -rf {} +
