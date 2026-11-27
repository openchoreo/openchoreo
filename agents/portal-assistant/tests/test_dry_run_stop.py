# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the check_authoring dry-run stop middleware."""

import json
from dataclasses import dataclass, field, replace

from langchain.messages import AIMessage, HumanMessage, ToolMessage

from src.agent.middleware.dry_run_stop import DryRunStopMiddleware


@dataclass
class _ToolCall:
    tool_call: dict


@dataclass
class _ModelRequest:
    tools: list = field(default_factory=lambda: ["tech-insights__dry-run-check", "tech-insights__list-checks"])
    model_settings: dict = field(default_factory=dict)
    messages: list = field(default_factory=lambda: [HumanMessage("test it on order-api")])

    def override(self, **kw):
        return replace(self, **kw)


def _tool_message(payload, status="success"):
    # langchain-mcp-adapters returns content blocks, not a plain string.
    return ToolMessage(
        content=[{"type": "text", "text": json.dumps(payload)}],
        tool_call_id="1",
        status=status,
    )


async def _call_tool(mw, name, result):
    async def handler(_request):
        return result

    return await mw.awrap_tool_call(_ToolCall({"name": name, "id": "1"}), handler)


async def _tools_seen_by_model(mw):
    seen = {}

    async def handler(request):
        seen["tools"] = request.tools
        return "response"

    await mw.awrap_model_call(_ModelRequest(), handler)
    return seen["tools"]


async def test_tools_stay_on_until_a_dry_run_has_a_result():
    mw = DryRunStopMiddleware()

    await _call_tool(mw, "tech-insights__get-entity-facts", _tool_message({"facts": {}}))

    assert await _tools_seen_by_model(mw) == ["tech-insights__dry-run-check", "tech-insights__list-checks"]


async def test_tools_are_removed_after_a_failed_dry_run():
    mw = DryRunStopMiddleware()

    result = _tool_message({"status": "failed", "passed": False})
    returned = await _call_tool(mw, "tech-insights__dry-run-check", result)

    assert returned is result  # the result itself passes through unchanged
    assert await _tools_seen_by_model(mw) == []


async def test_a_malformed_status_is_not_a_result():
    for status in (["failed"], {"value": "failed"}, None):
        mw = DryRunStopMiddleware()
        result = _tool_message({"status": status})

        assert await _call_tool(mw, "tech-insights__dry-run-check", result) is result
        assert await _tools_seen_by_model(mw) == ["tech-insights__dry-run-check", "tech-insights__list-checks"]


async def test_passed_and_not_applicable_also_end_the_loop():
    for status in ("passed", "not-applicable"):
        mw = DryRunStopMiddleware()
        await _call_tool(mw, "tech-insights__dry-run-check", _tool_message({"status": status}))
        assert await _tools_seen_by_model(mw) == [], status


def _error(text="InputError: No entity component:default/guess in the catalog."):
    return ToolMessage(content=text, tool_call_id="1", status="error")


async def test_three_failed_dry_run_attempts_end_the_loop():
    # e.g. no component was named and the model keeps guessing entity refs
    mw = DryRunStopMiddleware()

    for _ in range(2):
        await _call_tool(mw, "tech-insights__dry-run-check", _error())
    assert await _tools_seen_by_model(mw) == ["tech-insights__dry-run-check", "tech-insights__list-checks"]

    await _call_tool(mw, "tech-insights__dry-run-check", _error())
    assert await _tools_seen_by_model(mw) == []


async def test_six_tool_calls_end_the_loop():
    mw = DryRunStopMiddleware()

    for _ in range(5):
        await _call_tool(mw, "tech-insights__list-checks", _tool_message({"checks": []}))
    assert await _tools_seen_by_model(mw) == ["tech-insights__dry-run-check", "tech-insights__list-checks"]

    await _call_tool(mw, "tech-insights__list-checks", _tool_message({"checks": []}))
    assert await _tools_seen_by_model(mw) == []


async def test_an_errored_dry_run_keeps_tools_so_the_model_can_fix_it():
    mw = DryRunStopMiddleware()

    await _call_tool(
        mw,
        "tech-insights__dry-run-check",
        ToolMessage(
            content="InputError: rule.conditions is not valid JSON",
            tool_call_id="1",
            status="error",
        ),
    )

    assert await _tools_seen_by_model(mw) == ["tech-insights__dry-run-check", "tech-insights__list-checks"]


async def test_parallel_tool_calls_off_only_while_tools_are_bound():
    """Strict tools + parallel calls give malformed args on OpenAI, but the
    flag must not be sent once tools are gone (OpenAI rejects it then)."""
    mw = DryRunStopMiddleware()
    seen = []

    async def handler(request):
        seen.append(request.model_settings)
        return "response"

    await mw.awrap_model_call(_ModelRequest(), handler)
    await _call_tool(mw, "tech-insights__dry-run-check", _tool_message({"status": "failed"}))
    await mw.awrap_model_call(_ModelRequest(), handler)

    assert seen == [{"parallel_tool_calls": False}, {}]


async def test_malformed_tool_arguments_retry_once_without_tools():
    mw = DryRunStopMiddleware()
    calls = []

    async def handler(request):
        calls.append((request.tools, request.model_settings))
        if request.tools:
            raise json.JSONDecodeError("Unterminated string", '{"a": "', 6)
        return "final answer"

    assert await mw.awrap_model_call(_ModelRequest(), handler) == "final answer"
    assert calls == [
        (["tech-insights__dry-run-check", "tech-insights__list-checks"], {"parallel_tool_calls": False}),
        ([], {}),
    ]
    # Tools stay off for the rest of the turn.
    assert await _tools_seen_by_model(mw) == []


# ── validate-check: valid → reply, invalid → retry up to 3, can't → stop ─


def _validation(valid, problems=()):
    return _tool_message({"valid": valid, "problems": list(problems)})


async def _tools_now(mw, user_text, tools=("tech-insights__validate-check", "tech-insights__dry-run-check")):
    seen = {}

    async def handler(request):
        seen["tools"] = request.tools
        return "response"

    request = _ModelRequest(tools=list(tools), messages=[HumanMessage(user_text)])
    await mw.awrap_model_call(request, handler)
    return seen["tools"]


async def test_a_valid_rule_leaves_only_the_dry_run():
    mw = DryRunStopMiddleware()
    assert await _tools_now(mw, "allow internal repos too") == ["tech-insights__validate-check", "tech-insights__dry-run-check"]

    await _call_tool(mw, "tech-insights__validate-check", _validation(True))

    assert await _tools_now(mw, "allow internal repos too") == ["tech-insights__dry-run-check"]


async def test_invalid_results_allow_up_to_three_attempts():
    mw = DryRunStopMiddleware()
    problem = ["Unknown operator: equals (at /conditions/all/0/operator)"]

    for _ in range(2):
        await _call_tool(mw, "tech-insights__validate-check", _validation(False, problem))
        assert await _tools_now(mw, "allow internal repos too") == [
            "tech-insights__validate-check",
            "tech-insights__dry-run-check",
        ]

    await _call_tool(mw, "tech-insights__validate-check", _validation(False, problem))

    # Out of attempts: no more validation; the model shows its last rule
    # with the problems.
    assert await _tools_now(mw, "allow internal repos too") == ["tech-insights__dry-run-check"]


async def test_fixing_the_rule_after_problems_ends_the_loop():
    mw = DryRunStopMiddleware()

    await _call_tool(mw, "tech-insights__validate-check", _validation(False, ["Fact x isn't selected"]))
    await _call_tool(mw, "tech-insights__validate-check", _validation(True))

    assert await _tools_now(mw, "allow internal repos too") == ["tech-insights__dry-run-check"]


async def test_a_fixable_input_error_counts_as_an_attempt():
    mw = DryRunStopMiddleware()

    await _call_tool(mw, "tech-insights__validate-check", _error("InputError: rule.conditions is not valid JSON"))

    assert await _tools_now(mw, "allow internal repos too") == ["tech-insights__validate-check", "tech-insights__dry-run-check"]


async def test_no_retries_when_validation_is_not_allowed():
    mw = DryRunStopMiddleware()

    await _call_tool(
        mw,
        "tech-insights__validate-check",
        _error("NotAllowedError: You don't have the manage permission."),
    )

    assert await _tools_now(mw, "allow internal repos too") == ["tech-insights__dry-run-check"]


async def _tools_for(mw, messages):
    seen = {}

    async def handler(request):
        seen["tools"] = request.tools
        return "response"

    await mw.awrap_model_call(
        _ModelRequest(tools=["tech-insights__validate-check", "tech-insights__dry-run-check"], messages=messages), handler
    )
    return seen["tools"]


async def test_an_unapproved_edit_message_gets_no_tools(caplog):
    mw = DryRunStopMiddleware(tools_allowed=False)

    with caplog.at_level("INFO"):
        tools = await _tools_for(mw, [HumanMessage("Change it to 60 and test it on order-api")])

    assert tools == []
    assert "edit not approved — tools off for this message" in caplog.text


async def test_an_approved_edit_keeps_its_tools():
    mw = DryRunStopMiddleware(tools_allowed=True)

    tools = await _tools_for(
        mw,
        [
            HumanMessage("Change it to 60 and test it on order-api"),
            AIMessage(
                "Proposed rule: `githubDaysSinceLastPush lessThanInclusive 60`. Shall I change it?"
            ),
            HumanMessage("yes"),
        ],
    )

    assert tools == ["tech-insights__validate-check", "tech-insights__dry-run-check"]


async def test_a_new_check_keeps_its_tools_on_the_first_message():
    mw = DryRunStopMiddleware()

    tools = await _tools_for(mw, [HumanMessage("Repos must be private, test it on order-api")])

    assert tools == ["tech-insights__validate-check", "tech-insights__dry-run-check"]


async def test_the_rewrite_of_an_unapproved_reply_gets_no_tools():
    # Same message: the reply skipped Step 1, and the orchestrator asks the
    # model to write it again. The user still hasn't confirmed anything.
    mw = DryRunStopMiddleware(tools_allowed=False)
    first = [HumanMessage("Change it to 60 and test it on order-api")]
    await _tools_for(mw, first)

    tools = await _tools_for(
        mw,
        [
            *first,
            AIMessage('```json\n{"all":[]}\n```'),
            HumanMessage("[Automatic check — not written by the user] … confirmation …"),
        ],
    )

    assert tools == []


async def test_a_test_only_message_gets_dry_run_check_alone():
    mw = DryRunStopMiddleware(dry_run_only=True)

    assert await _tools_for(mw, [HumanMessage("test it on order-api")]) == ["tech-insights__dry-run-check"]


async def test_a_test_only_message_gets_no_tools_after_the_result():
    mw = DryRunStopMiddleware(dry_run_only=True)

    await _call_tool(mw, "tech-insights__dry-run-check", _tool_message({"status": "passed", "passed": True}))

    assert await _tools_for(mw, [HumanMessage("test it on order-api")]) == []
