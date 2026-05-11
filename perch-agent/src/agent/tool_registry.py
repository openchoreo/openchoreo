# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging

from langchain_core.tools import BaseTool

logger = logging.getLogger(__name__)

# Verbs at the start of a tool name that strongly signal a mutating operation.
WRITE_VERB_PREFIXES = (
    "create_",
    "update_",
    "delete_",
    "patch_",
    "apply_",
    "trigger_",
)

# Embedded write-verb fragments — catches naming patterns like
# "pe_create_workflow" where the toolset prefixes the verb.
WRITE_VERB_SUBSTRINGS = (
    "_create_",
    "_update_",
    "_delete_",
    "_patch_",
    "_apply_",
    "_trigger_",
)

# Tools whose name matches a write prefix but are actually read-only.
# Add new false-positives here as MCP servers introduce them. The ones below
# all share a write authz action with their corresponding mutating tool but
# only return a JSON Schema for the user to fill in.
READ_OVERRIDES: frozenset[str] = frozenset(
    {
        "get_component_type_creation_schema",
        "get_cluster_component_type_creation_schema",
        "get_trait_creation_schema",
    }
)

# Description-keyword fallback for tools whose names don't match the prefix
# patterns. Verbs are normalised to their third-person forms because tool
# descriptions tend to read "Creates a new ...", "Updates ...", etc.
WRITE_KEYWORDS = (
    "creates",
    "updates",
    "deletes",
    "modifies",
    "removes",
    "applies",
    "provisions",
    "triggers",
)


def is_mutating(tool: BaseTool) -> bool:
    """Return True if invoking `tool` should be treated as a write requiring user
    confirmation.

    Detection is intentionally heuristic so this works uniformly across MCP
    servers — including third-party MCPs we adopt later that won't carry an
    OpenChoreo-specific server-side flag. The decision is:

      1. Explicit READ_OVERRIDES win (returns False).
      2. Name prefix or substring matches a write verb → True.
      3. Name prefix matches a read verb → False. **This runs before the
         description-keyword fallback** because a description like
         "applies filtering" or "triggers a search" tripped the keyword
         fallback for clearly read-only ``query_*`` tools, refusing them
         as mutations at WriteGuard time and burning two ~30s LLM
         round-trips per turn.
      4. Description-keyword fallback → True (only reaches here for
         names with no prefix in either set, e.g. ``analyze_*``).
      5. Default to True on ambiguity. The failure mode of a false
         positive is "user is asked to confirm a read"; the failure
         mode of a false negative is "agent silently writes without
         preview." The asymmetry justifies erring toward confirmation.

    See also: services.ErrDryRunNotSupported on the Go side — even if the agent
    decides a tool is mutating and dry-runs it, the underlying service may not
    yet support dry-run, in which case it returns the typed error rather than
    silently executing.
    """
    if tool.name in READ_OVERRIDES:
        return False

    if any(tool.name.startswith(p) for p in WRITE_VERB_PREFIXES):
        return True

    if any(s in tool.name for s in WRITE_VERB_SUBSTRINGS):
        return True

    # Read-prefixed names ("list_", "get_", "describe_", "query_", ...) are
    # the common case that should NOT default-true. ``query_`` is load-bearing:
    # the observer MCP exposes query_component_logs / query_trace_spans /
    # query_traces / query_resource_metrics / query_http_metrics /
    # query_alerts / query_incidents / query_workflow_logs — every one of
    # them is read-only. Without ``query_`` here, the description fallback
    # below misclassifies any whose description contains a write verb (e.g.
    # "applies filtering", "triggers a search"), and the WriteGuard then
    # refuses the call as a mutation when the user message has no write intent.
    READ_PREFIXES = (
        "list_",
        "get_",
        "describe_",
        "view_",
        "show_",
        "fetch_",
        "query_",
    )
    if any(tool.name.startswith(p) for p in READ_PREFIXES):
        return False

    # Description-keyword fallback — only reaches here for names that didn't
    # match either prefix set (e.g. analyze_runtime_state).
    desc = (tool.description or "").lower()
    if any(v in desc for v in WRITE_KEYWORDS):
        return True

    # Genuinely unknown — be safe.
    return True


def log_classification_summary(tools: list[BaseTool]) -> None:
    """Log the (mutating, read-only) split for the loaded tool catalog.

    Called from the FastAPI lifespan once startup completes. Helps ops
    spot the case where a freshly-introduced MCP write tool isn't matched
    by any of the heuristics above and would silently bypass WriteGuard.
    Look for a tool you know is mutating appearing under "read-only" — if
    it does, add a prefix/keyword/READ_OVERRIDE entry to this module.
    """
    mutating: list[str] = []
    read_only: list[str] = []
    for t in tools:
        (mutating if is_mutating(t) else read_only).append(t.name)
    mutating.sort()
    read_only.sort()
    logger.info(
        "Tool classification: %d mutating, %d read-only",
        len(mutating),
        len(read_only),
    )
    logger.info("  mutating: %s", ", ".join(mutating) if mutating else "(none)")
    logger.info("  read-only: %s", ", ".join(read_only) if read_only else "(none)")
