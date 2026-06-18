# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the auth layer: bearer-token extraction, the ``require_authn``
dependency (disabled / missing-token / valid / invalid paths), entitlement
extraction from JWT claims, and authorization (both the ``AuthorizationChecker``
dependency with a mocked ``AuthzClient`` and ``AuthzClient.evaluate``'s HTTP
status-code mapping with a mocked ``httpx`` client).
"""

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import HTTPException

from src.auth import dependencies as deps
from src.auth.authz_client import AuthzClient
from src.auth.authz_models import (
    Decision,
    EvaluateRequest,
    Resource,
    ResourceHierarchy,
    SubjectContext,
)
from src.auth.dependencies import (
    AuthorizationChecker,
    ReportAuthorizationChecker,
    extract_bearer_token,
    extract_subject_context_from_claims,
    require_authn,
)
from src.auth.jwt import DisabledJWTValidator, JWTValidationError


def _request(headers=None, query_params=None):
    return SimpleNamespace(
        headers=headers or {},
        query_params=query_params or {},
        state=SimpleNamespace(),
    )


def _subject(values=None):
    return SubjectContext(
        type="user",
        entitlementClaim="sub",
        entitlementValues=values or ["user-1"],
    )


# ------------------------------------------------------- bearer token extraction


def test_extract_bearer_token_valid():
    assert extract_bearer_token(_request({"Authorization": "Bearer abc.def"})) == "abc.def"


def test_extract_bearer_token_missing_header():
    assert extract_bearer_token(_request({})) is None


def test_extract_bearer_token_wrong_scheme():
    assert extract_bearer_token(_request({"Authorization": "Basic xyz"})) is None


def test_extract_bearer_token_scheme_is_case_insensitive():
    assert extract_bearer_token(_request({"Authorization": "bearer abc"})) == "abc"


def test_extract_bearer_token_malformed_single_part():
    assert extract_bearer_token(_request({"Authorization": "Bearertoken"})) is None


# ---------------------------------------------------------------- require_authn


@pytest.mark.asyncio
async def test_require_authn_disabled_validator_is_500():
    with (
        patch.object(deps, "get_jwt_validator", return_value=DisabledJWTValidator()),
        pytest.raises(HTTPException) as exc,
    ):
        await require_authn(_request({"Authorization": "Bearer tok"}))
    assert exc.value.status_code == 500


@pytest.mark.asyncio
async def test_require_authn_missing_token_is_401():
    validator = MagicMock()  # not a DisabledJWTValidator
    with (
        patch.object(deps, "get_jwt_validator", return_value=validator),
        pytest.raises(HTTPException) as exc,
    ):
        await require_authn(_request({}))
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_require_authn_invalid_token_is_401():
    validator = MagicMock()
    validator.validate = MagicMock(side_effect=JWTValidationError("bad"))
    with (
        patch.object(deps, "get_jwt_validator", return_value=validator),
        pytest.raises(HTTPException) as exc,
    ):
        await require_authn(_request({"Authorization": "Bearer tok"}))
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_require_authn_valid_token_returns_subject_and_stashes_token():
    validator = MagicMock()
    validator.validate = MagicMock(return_value={"sub": "user-9"})
    req = _request({"Authorization": "Bearer tok"})

    with (
        patch.object(deps, "get_jwt_validator", return_value=validator),
        patch.object(deps, "_get_subject_types", return_value=[]),
    ):
        subject = await require_authn(req)

    assert subject.entitlement_values == ["user-9"]
    assert req.state.bearer_token == "tok"


# ------------------------------------------------------- entitlement extraction


def test_extract_entitlements_list_filters_falsy():
    assert deps._extract_entitlements({"groups": ["a", "b", ""]}, "groups") == ["a", "b"]


def test_extract_entitlements_scalar_is_wrapped():
    assert deps._extract_entitlements({"sub": "u1"}, "sub") == ["u1"]


def test_extract_entitlements_missing_claim_is_none():
    assert deps._extract_entitlements({}, "groups") is None


def test_extract_entitlements_empty_list_is_none():
    assert deps._extract_entitlements({"groups": []}, "groups") is None


def test_extract_entitlements_empty_scalar_is_none():
    assert deps._extract_entitlements({"groups": ""}, "groups") is None


def test_get_jwt_claim_finds_jwt_mechanism():
    cfg = {"auth_mechanisms": [{"type": "jwt", "entitlement": {"claim": "groups"}}]}
    assert deps._get_jwt_claim(cfg) == "groups"


def test_get_jwt_claim_returns_none_without_jwt_mechanism():
    assert deps._get_jwt_claim({"auth_mechanisms": [{"type": "apikey"}]}) is None


def test_extract_subject_context_matches_configured_type():
    subject_types = [
        {
            "type": "group",
            "auth_mechanisms": [{"type": "jwt", "entitlement": {"claim": "groups"}}],
        }
    ]
    with patch.object(deps, "_get_subject_types", return_value=subject_types):
        ctx = extract_subject_context_from_claims({"groups": ["team-a", "team-b"]})

    assert ctx.type == "group"
    assert ctx.entitlement_claim == "groups"
    assert ctx.entitlement_values == ["team-a", "team-b"]


def test_extract_subject_context_falls_back_to_sub():
    with patch.object(deps, "_get_subject_types", return_value=[]):
        ctx = extract_subject_context_from_claims({"sub": "user-9"})

    assert ctx.type == "user"
    assert ctx.entitlement_claim == "sub"
    assert ctx.entitlement_values == ["user-9"]


# ----------------------------------------- AuthorizationChecker (mocked client)


@pytest.mark.asyncio
async def test_authorization_checker_allows_and_forwards_request():
    client = AsyncMock()
    client.evaluate = AsyncMock(return_value=Decision(decision=True))
    checker = AuthorizationChecker(action="finopsreport:view", resource_type="finopsreport")
    subject = _subject()
    req = _request({"Authorization": "Bearer tok"})

    with patch.object(deps, "get_authz_client", return_value=client):
        result = await checker(req, subject)

    assert result is subject
    sent_request, sent_token = client.evaluate.await_args.args
    assert sent_request.action == "finopsreport:view"
    assert sent_request.resource.type == "finopsreport"
    assert sent_token == "tok"


@pytest.mark.asyncio
async def test_authorization_checker_denies_with_403():
    client = AsyncMock()
    client.evaluate = AsyncMock(return_value=Decision(decision=False))
    checker = AuthorizationChecker(action="finopsreport:view", resource_type="finopsreport")

    with (
        patch.object(deps, "get_authz_client", return_value=client),
        pytest.raises(HTTPException) as exc,
    ):
        await checker(_request({"Authorization": "Bearer tok"}), _subject())

    assert exc.value.status_code == 403


@pytest.mark.asyncio
async def test_report_checker_extracts_hierarchy_from_query_params():
    checker = ReportAuthorizationChecker(action="x", resource_type="y")
    req = _request(query_params={"project": "p1", "namespace": "n1"})

    hierarchy = await checker._extract_hierarchy(req)

    assert hierarchy.project == "p1"
    assert hierarchy.namespace == "n1"


# ------------------------------------------- AuthzClient.evaluate status mapping


def _make_authz_client():
    client = AuthzClient(base_url="http://authz", timeout=5.0)
    client._client = AsyncMock()
    return client


def _http_response(status_code, json_data=None):
    resp = MagicMock()
    resp.status_code = status_code
    resp.json = MagicMock(return_value=json_data)
    return resp


def _eval_request():
    return EvaluateRequest(
        subjectContext=_subject(),
        resource=Resource(type="finopsreport", hierarchy=ResourceHierarchy()),
        action="finopsreport:view",
    )


@pytest.mark.asyncio
async def test_authz_client_returns_allow_decision_on_200():
    client = _make_authz_client()
    client._client.post = AsyncMock(return_value=_http_response(200, [{"decision": True}]))

    decision = await client.evaluate(_eval_request(), "tok")

    assert decision.decision is True


@pytest.mark.asyncio
async def test_authz_client_returns_deny_decision_on_200():
    client = _make_authz_client()
    client._client.post = AsyncMock(return_value=_http_response(200, [{"decision": False}]))

    decision = await client.evaluate(_eval_request(), "tok")

    assert decision.decision is False


@pytest.mark.asyncio
async def test_authz_client_maps_401():
    client = _make_authz_client()
    client._client.post = AsyncMock(return_value=_http_response(401))

    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request(), "tok")
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_authz_client_maps_403():
    client = _make_authz_client()
    client._client.post = AsyncMock(return_value=_http_response(403))

    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request(), "tok")
    assert exc.value.status_code == 403


@pytest.mark.asyncio
async def test_authz_client_empty_decisions_is_500():
    client = _make_authz_client()
    client._client.post = AsyncMock(return_value=_http_response(200, []))

    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request(), "tok")
    assert exc.value.status_code == 500
