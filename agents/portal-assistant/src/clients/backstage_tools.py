# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""The Backstage check-authoring tools, by the exact names the agent uses.

Backstage's mcp-actions backend serves them with namespaced names, such as
``tech-insights.list-checks``. Model tool names can't hold a dot, so
clients/mcp.py renames each Backstage tool with ``agent_name`` and the agent
matches only these names, never a bare name another MCP server could share.
"""

import copy
import json
from typing import Any

from langchain_core.tools import BaseTool

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

# The model's tools that take a rule. On OpenAI's Chat Completions API (a
# base_url override, e.g. Azure OpenAI) langchain binds tools with strict
# function calling, which allows no free-form object and makes every field
# required: OpenAI refuses the whole request for Backstage's
# ``rule.conditions`` (any JSON object), and an optional field would have to
# be sent. The Responses API doesn't bind tools strict; this form works on
# both. The model gets ``conditions`` as a JSON string, parsed back before
# the call reaches Backstage, and not the fields only code sends. Code's own
# calls pass ``conditions`` as an object and the other fields as before.
_RULE_PATHS = {VALIDATE_CHECK: ("rule",), DRY_RUN_CHECK: ("check", "rule")}
_CODE_ONLY_FIELDS = {VALIDATE_CHECK: ("id",)}
_CONDITIONS_AS_STRING = {
    "type": "string",
    "description": (
        "json-rules-engine conditions as a JSON string, e.g. "
        '{"all":[{"fact":"githubIsArchived","operator":"equal","value":false}]}'
    ),
}


def _at(node: Any, path: tuple[str, ...], key: str) -> Any:
    """``node[key]`` at each step of ``path``, or None when it isn't there."""
    for step in path:
        node = node.get(key, {}).get(step) if key else node.get(step)
        if not isinstance(node, dict):
            return None
    return node


def strict_for_model(tool: BaseTool) -> None:
    """Make a Backstage tool's schema valid for strict function calling, as
    the model sees it (see _RULE_PATHS). Other tools are left as they are."""
    path = _RULE_PATHS.get(tool.name)
    if path is None or not isinstance(tool.args_schema, dict):
        return
    schema = copy.deepcopy(tool.args_schema)
    rule = _at(schema, path, "properties")
    if rule is None or "conditions" not in rule.get("properties", {}):
        return
    rule["properties"]["conditions"] = _CONDITIONS_AS_STRING
    for field in _CODE_ONLY_FIELDS.get(tool.name, ()):
        schema.get("properties", {}).pop(field, None)
        if field in schema.get("required", []):
            schema["required"].remove(field)
    tool.args_schema = schema

    call = tool.coroutine

    async def with_conditions_parsed(**kwargs: Any) -> Any:
        rule = _at(kwargs, path, "")
        if rule is not None and isinstance(rule.get("conditions"), str):
            try:
                conditions = json.loads(rule["conditions"])
            except ValueError:
                conditions = None  # sent as is: Backstage says what's wrong
            if conditions is not None:
                kwargs = copy.deepcopy(kwargs)
                _at(kwargs, path, "")["conditions"] = conditions
        return await call(**kwargs)

    tool.coroutine = with_conditions_parsed
