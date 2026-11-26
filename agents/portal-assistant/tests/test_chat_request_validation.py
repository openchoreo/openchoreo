# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ChatRequest validation — particularly the new total-content cap."""
import pytest
from pydantic import ValidationError

from src.api.agent_routes import (
    _TOTAL_CONTENT_LIMIT,
    ChatMessage,
    ChatRequest,
    ChatScope,
    PrefetchedLogEntry,
)


def test_short_request_passes():
    req = ChatRequest(
        messages=[ChatMessage(role="user", content="hi")],
    )
    assert len(req.messages) == 1


def test_per_message_cap_enforced():
    with pytest.raises(ValidationError):
        ChatMessage(role="user", content="x" * 10_001)


def test_total_content_cap_rejects_overage():
    # Each message at 10k → 7 messages = 70k > 60k limit → reject.
    msgs = [ChatMessage(role="user", content="x" * 10_000) for _ in range(7)]
    with pytest.raises(ValidationError) as excinfo:
        ChatRequest(messages=msgs)
    assert "exceeds limit" in str(excinfo.value)


def test_total_content_cap_at_boundary():
    # Per-message cap is 10k, so split the total across multiple messages.
    per_msg = 10_000
    msgs = [
        ChatMessage(role="user", content="x" * per_msg)
        for _ in range(_TOTAL_CONTENT_LIMIT // per_msg)
    ]
    req = ChatRequest(messages=msgs)
    assert sum(len(m.content) for m in req.messages) == _TOTAL_CONTENT_LIMIT


def test_extra_keys_rejected_by_chat_message():
    # ChatMessage has model_config={"extra": "forbid"}.
    with pytest.raises(ValidationError):
        ChatMessage(role="user", content="hi", extra_field="bad")


def test_message_count_cap_enforced():
    msgs = [ChatMessage(role="user", content="x") for _ in range(51)]
    with pytest.raises(ValidationError):
        ChatRequest(messages=msgs)


def test_prefetched_logs_accepts_camel_case_alias():
    """The frontend serializes ``prefetchedLogs``; the agent stores it
    as ``prefetched_logs``. Round-trip through the alias is the
    contract — break this and chats from Backstage stop seeing the
    pre-loaded rows even though pydantic happily ignores the field."""
    scope = ChatScope.model_validate(
        {
            "prefetchedLogs": [
                {
                    "timestamp": "2026-05-14T10:00:00Z",
                    "level": "ERROR",
                    "message": "db connection refused",
                    "componentName": "order-api",
                },
            ],
        }
    )
    assert scope.prefetched_logs is not None
    assert len(scope.prefetched_logs) == 1
    assert scope.prefetched_logs[0].message == "db connection refused"
    assert scope.prefetched_logs[0].component_name == "order-api"


def test_prefetched_logs_message_required():
    """A row with no message is meaningless — the agent has nothing to
    show the user from it. Forcing ``message`` to be present means
    the frontend trimmer's empty-message bug surfaces here instead
    of producing a useless prompt."""
    with pytest.raises(ValidationError):
        PrefetchedLogEntry(timestamp="2026-05-14T10:00:00Z", level="ERROR")


def test_prefetched_logs_row_count_capped():
    """Per the ChatScope.prefetched_logs Field(max_length=50). Without
    this, a malicious / buggy client could flood the prompt context
    without tripping the per-row size cap."""
    rows = [{"message": "x"} for _ in range(51)]
    with pytest.raises(ValidationError):
        ChatScope.model_validate({"prefetchedLogs": rows})


def test_total_content_counter_includes_prefetched_logs():
    """The total-content validator must count strings nested inside
    list-of-models. Earlier the validator only walked list-of-string
    items; a 50-row prefetched_logs list at the per-row cap would
    have been invisible to the budget check."""
    # Each row.message=2000 chars × 50 rows = 100k chars from
    # prefetched_logs alone → above the 60k limit when added to even
    # one short ChatMessage.
    huge_rows = [{"message": "x" * 2000} for _ in range(50)]
    msgs = [ChatMessage(role="user", content="hi")]
    with pytest.raises(ValidationError) as excinfo:
        ChatRequest.model_validate(
            {
                "messages": [m.model_dump() for m in msgs],
                "scope": {"prefetchedLogs": huge_rows},
            }
        )
    assert "exceeds limit" in str(excinfo.value)


def test_available_fact_retrievers_accepts_camel_case_alias():
    """Frontend sends availableFactRetrievers/selectedFactIds (camelCase);
    the agent stores them as snake_case. Round-trip through the alias is
    the contract the check_authoring prompt branch depends on."""
    scope = ChatScope.model_validate(
        {
            "caseType": "check_authoring",
            "availableFactRetrievers": [
                {
                    "id": "githubRepositoryFactRetriever",
                    "facts": [
                        {
                            "name": "githubIsArchived",
                            "type": "boolean",
                            "description": "repo is archived",
                        }
                    ],
                },
            ],
            "selectedFactIds": ["githubRepositoryFactRetriever"],
        }
    )
    assert scope.available_fact_retrievers[0].id == "githubRepositoryFactRetriever"
    assert scope.selected_fact_ids == ["githubRepositoryFactRetriever"]


def test_editing_check_fields_accept_camel_case_alias():
    """The edit-dialog launcher sends checkId/checkName/checkDescription/
    currentRule (camelCase); the check_authoring prompt's editing section
    reads the snake_case names. A checkSolution from an older launcher is
    accepted and ignored."""
    rule = '{"all":[{"fact":"githubVisibility","operator":"equal","value":"public"}]}'
    scope = ChatScope.model_validate(
        {
            "caseType": "check_authoring",
            "checkId": "abc-123",
            "checkName": "Repo must be public",
            "checkDescription": "Fails if the repository is not public",
            "checkSolution": "Make the repository public",
            "currentRule": rule,
        }
    )
    assert scope.check_id == "abc-123"
    assert scope.check_name == "Repo must be public"
    assert scope.check_description == "Fails if the repository is not public"
    assert not hasattr(scope, "check_solution")
    assert scope.current_rule == rule


def test_current_rule_length_capped():
    """Per the ChatScope.current_rule Field(max_length=8000) — the rule is
    rendered into the system prompt, so it must stay bounded."""
    with pytest.raises(ValidationError):
        ChatScope.model_validate({"currentRule": "x" * 8001})


def test_total_content_counter_includes_nested_fact_retrievers():
    """available_fact_retrievers nests two levels deep (retriever -> facts
    -> description) — the recursive walker must reach that, or oversized
    content here is invisible to the 60k budget check."""
    one_fact = {"name": "f", "type": "boolean", "description": "x" * 512}
    retriever = {"id": "r", "facts": [one_fact] * 100}
    with pytest.raises(ValidationError) as excinfo:
        ChatRequest.model_validate(
            {
                "messages": [{"role": "user", "content": "hi"}],
                "scope": {"availableFactRetrievers": [retriever, retriever]},
            }
        )
    assert "exceeds limit" in str(excinfo.value)


def test_fact_retriever_keeps_the_kinds_it_collects_for():
    scope = ChatScope.model_validate(
        {
            "caseType": "check_authoring",
            "availableFactRetrievers": [{"id": "r", "kinds": ["component"], "facts": []}],
        }
    )
    assert scope.model_dump(exclude_none=True)["available_fact_retrievers"][0]["kinds"] == ["component"]


# ── the Backstage user token reaches the chat ──────────────────────────


def _chat_kwargs(monkeypatch, headers):
    """The keyword arguments the /chat route passes to stream_chat."""
    import asyncio
    from types import SimpleNamespace

    from src.api import agent_routes

    seen = {}

    def fake_stream_chat(**kwargs):
        seen.update(kwargs)

        async def events():
            yield ""

        return events()

    monkeypatch.setattr(agent_routes, "stream_chat", fake_stream_chat)
    http_request = SimpleNamespace(
        state=SimpleNamespace(bearer_token="openchoreo-token", user_sub="u"),
        headers=headers,
    )
    request = ChatRequest(messages=[ChatMessage(role="user", content="hi")])
    asyncio.run(agent_routes.chat(request, http_request, None, None))
    return seen


def test_chat_passes_the_backstage_user_token_on(monkeypatch):
    seen = _chat_kwargs(monkeypatch, {"x-backstage-user-token": "issued-for-user"})

    assert seen["backstage_token"] == "issued-for-user"
    assert seen["token"] == "openchoreo-token"


def test_chat_without_the_header_has_no_backstage_token(monkeypatch):
    assert _chat_kwargs(monkeypatch, {})["backstage_token"] is None


def test_scope_carries_the_welcome_flag():
    from src.api.agent_routes import ChatScope

    assert ChatScope.model_validate({"caseType": "check_authoring", "welcome": True}).welcome is True
    assert ChatScope.model_validate({"caseType": "check_authoring"}).welcome is None


def test_scope_carries_the_users_experience():
    scope = ChatScope.model_validate({"caseType": "check_authoring", "experience": "experienced"})
    assert scope.experience == "experienced"
    with pytest.raises(ValidationError):
        ChatScope.model_validate({"caseType": "check_authoring", "experience": "expert"})
