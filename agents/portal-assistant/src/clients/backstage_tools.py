# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""The Backstage check-authoring tools, by the exact names the agent uses.

Backstage's mcp-actions backend serves them with namespaced names, such as
``tech-insights.list-checks``. Model tool names can't hold a dot, so
clients/mcp.py renames each Backstage tool with ``agent_name`` and the agent
matches only these names, never a bare name another MCP server could share.
"""

NAMESPACE = "tech-insights"


def agent_name(mcp_name: str) -> str:
    """``tech-insights.list-checks`` → ``tech-insights__list-checks``."""
    return mcp_name.replace(".", "__", 1)


def _tool(action: str) -> str:
    return agent_name(f"{NAMESPACE}.{action}")


LIST_CHECKS = _tool("list-checks")
GET_CHECK = _tool("get-check")
LIST_FACT_SCHEMAS = _tool("list-fact-schemas")
GET_ENTITY_FACTS = _tool("get-entity-facts")
DRY_RUN_CHECK = _tool("dry-run-check")
VALIDATE_CHECK = _tool("validate-check")
CREATE_CHECK = _tool("create-check")
UPDATE_CHECK = _tool("update-check")
PUBLISH_CHECK = _tool("publish-check")
UNPUBLISH_CHECK = _tool("unpublish-check")

# What the model may call: these only read, or evaluate a rule in memory.
READ_TOOLS = frozenset(
    {LIST_CHECKS, GET_CHECK, LIST_FACT_SCHEMAS, GET_ENTITY_FACTS, DRY_RUN_CHECK, VALIDATE_CHECK}
)
# Saved only by the confirm endpoint, as the user, never by the model.
WRITE_TOOLS = frozenset({CREATE_CHECK, UPDATE_CHECK, PUBLISH_CHECK, UNPUBLISH_CHECK})
