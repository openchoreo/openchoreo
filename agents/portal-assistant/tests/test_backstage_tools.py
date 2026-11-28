# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the Backstage tools' schemas under OpenAI strict function calling."""

import json

import pytest
from langchain_core.tools import StructuredTool
from langchain_openai import ChatOpenAI

from src.clients.backstage_tools import DRY_RUN_CHECK, VALIDATE_CHECK, strict_for_model

# The input schemas Backstage serves for these tools, as its actions registry
# builds them (zod-to-json-schema over the actions' zod schemas).
_CONDITIONS = {
    "type": "object",
    "additionalProperties": {},
    "description": "json-rules-engine conditions, e.g. "
    '{"all":[{"fact":"githubIsArchived","operator":"equal","value":false}]}',
}
_RULE = {
    "type": "object",
    "properties": {"conditions": _CONDITIONS},
    "required": ["conditions"],
    "additionalProperties": False,
    "description": "The json-rules-engine rule",
}
_FACT_IDS = {
    "type": "array",
    "items": {"type": "string"},
    "minItems": 1,
    "description": "Fact retriever ids the rule reads facts from (see list-fact-schemas)",
}
_SCHEMAS = {
    VALIDATE_CHECK: {
        "type": "object",
        "properties": {
            "id": {
                "type": "string",
                "minLength": 1,
                "description": "The saved check this rule is for, when editing one (from list-checks)",
            },
            "factIds": _FACT_IDS,
            "rule": _RULE,
        },
        "required": ["factIds", "rule"],
        "additionalProperties": False,
        "$schema": "http://json-schema.org/draft-07/schema#",
    },
    DRY_RUN_CHECK: {
        "type": "object",
        "properties": {
            "entityRef": {"type": "string", "minLength": 1, "description": "Entity ref"},
            "check": {
                "type": "object",
                "properties": {
                    "name": {"type": "string", "minLength": 1, "description": "Short check name"},
                    "description": {"type": "string", "description": "What the check verifies"},
                    "factIds": _FACT_IDS,
                    "rule": _RULE,
                },
                "required": ["name", "description", "factIds", "rule"],
                "additionalProperties": False,
            },
        },
        "required": ["entityRef", "check"],
        "additionalProperties": False,
        "$schema": "http://json-schema.org/draft-07/schema#",
    },
}
_RULE_OBJECT = {"all": [{"fact": "githubIsArchived", "operator": "equal", "value": False}]}


def _tool(name, calls):
    async def call(**kwargs):
        calls.append(kwargs)
        return "{}", None

    return StructuredTool(
        name=name,
        description="d",
        args_schema=json.loads(json.dumps(_SCHEMAS[name])),
        coroutine=call,
        response_format="content_and_artifact",
    )


def _strict_parameters(tool):
    """The tool's parameters as the agent's model is bound with them on Chat
    Completions (a base_url override): create_agent sets strict=True there."""
    bound = ChatOpenAI(model="gpt-5-mini", api_key="test").bind_tools([tool], strict=True)
    (spec,) = bound.kwargs["tools"]
    assert spec["function"]["strict"] is True
    return spec["function"]["parameters"]


def _strict_problems(node, path=""):
    """What OpenAI's strict mode refuses or empties: an optional field, or an
    object with no listed properties (strict mode forbids any key in it)."""
    if not isinstance(node, dict):
        return []
    problems = []
    if node.get("type") == "object":
        properties = node.get("properties") or {}
        if not properties:
            problems.append(f"{path or '/'} takes no keys")
        missing = sorted(set(properties) - set(node.get("required") or []))
        if missing:
            problems.append(f"{path or '/'} has optional {missing}")
        for key, child in properties.items():
            problems += _strict_problems(child, f"{path}/{key}")
    if node.get("type") == "array":
        problems += _strict_problems(node.get("items"), f"{path}[]")
    return problems


@pytest.mark.parametrize(
    ("name", "problems"),
    [
        (VALIDATE_CHECK, ["/rule/conditions takes no keys"]),
        (DRY_RUN_CHECK, ["/check/rule/conditions takes no keys"]),
    ],
)
def test_backstages_schema_doesnt_hold_under_strict_mode(name, problems):
    # The cause: strict mode turns the free-form conditions into an object
    # that takes no keys, which OpenAI refuses (400 invalid_function_parameters).
    assert _strict_problems(_strict_parameters(_tool(name, []))) == problems


def test_strict_mode_would_make_the_model_send_a_check_id():
    # Strict mode makes every field required, so the model would have to
    # invent an id for a new check, which Backstage then refuses.
    assert "id" in _strict_parameters(_tool(VALIDATE_CHECK, []))["required"]


@pytest.mark.parametrize("name", [VALIDATE_CHECK, DRY_RUN_CHECK])
def test_the_models_schema_holds_under_strict_mode(name):
    tool = _tool(name, [])

    strict_for_model(tool)

    parameters = _strict_parameters(tool)
    assert _strict_problems(parameters) == []
    assert "id" not in parameters["properties"]


async def test_the_models_conditions_reach_backstage_as_an_object():
    calls = []
    tool = _tool(DRY_RUN_CHECK, calls)
    strict_for_model(tool)
    check = {"name": "n", "description": "", "factIds": ["r"]}
    args = {
        "entityRef": "component:default/a",
        "check": {**check, "rule": {"conditions": json.dumps(_RULE_OBJECT)}},
    }

    await tool.ainvoke({"type": "tool_call", "id": "1", "name": tool.name, "args": args})

    assert calls == [{**args, "check": {**check, "rule": {"conditions": _RULE_OBJECT}}}]


async def test_codes_own_call_is_sent_as_it_is():
    # The edit card sends the check's id and the conditions as an object.
    calls = []
    tool = _tool(VALIDATE_CHECK, calls)
    strict_for_model(tool)
    args = {"id": "c1", "factIds": ["r"], "rule": {"conditions": _RULE_OBJECT}}

    await tool.ainvoke({"type": "tool_call", "id": "1", "name": tool.name, "args": args})

    assert calls == [args]


async def test_conditions_that_arent_json_go_to_backstage_to_report():
    calls = []
    tool = _tool(VALIDATE_CHECK, calls)
    strict_for_model(tool)
    args = {"factIds": ["r"], "rule": {"conditions": "not json"}}

    await tool.ainvoke({"type": "tool_call", "id": "1", "name": tool.name, "args": args})

    assert calls == [args]


def test_other_tools_are_left_as_they_are():
    tool = StructuredTool(
        name="tech-insights__get-check",
        description="d",
        args_schema={"type": "object", "properties": {"id": {"type": "string"}}},
        coroutine=lambda **_: None,
    )
    schema, call = tool.args_schema, tool.coroutine

    strict_for_model(tool)

    assert (tool.args_schema, tool.coroutine) == (schema, call)
