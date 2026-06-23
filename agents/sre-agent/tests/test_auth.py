# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the auth stack: bearer extraction, JWT, dependencies, authz client."""

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import jwt as pyjwt
import pytest
from fastapi import HTTPException

import src.auth.dependencies as deps
import src.auth.jwt as jwt_module
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
    _extract_entitlements,
    extract_bearer_token,
    extract_subject_context_from_claims,
    require_authn,
)
from src.auth.jwt import (
    DisabledJWTValidator,
    JWTValidationError,
    JWTValidator,
    get_jwt_validator,
)


def _request(headers=None, path_params=None, body=None):
    return SimpleNamespace(
        headers=headers or {},
        state=SimpleNamespace(),
        path_params=path_params or {},
        json=AsyncMock(return_value=body or {}),
    )


# --------------------------------------------------------- bearer token


def test_extract_bearer_token_valid():
    assert extract_bearer_token(_request({"Authorization": "Bearer abc"})) == "abc"


def test_extract_bearer_token_missing_header():
    assert extract_bearer_token(_request({})) is None


def test_extract_bearer_token_wrong_scheme():
    assert extract_bearer_token(_request({"Authorization": "Basic abc"})) is None


def test_extract_bearer_token_is_case_insensitive_scheme():
    assert extract_bearer_token(_request({"Authorization": "bearer abc"})) == "abc"


def test_extract_bearer_token_malformed():
    assert extract_bearer_token(_request({"Authorization": "Bearer"})) is None


# ----------------------------------------------------- entitlements


def test_extract_entitlements_missing_claim_returns_none():
    assert _extract_entitlements({}, "groups") is None


def test_extract_entitlements_list_filters_falsy_and_stringifies():
    assert _extract_entitlements({"groups": ["a", "", 7]}, "groups") == ["a", "7"]


def test_extract_entitlements_scalar_wrapped():
    assert _extract_entitlements({"groups": "team"}, "groups") == ["team"]


def test_extract_entitlements_empty_value_returns_empty_list():
    assert _extract_entitlements({"groups": ""}, "groups") == []


# ------------------------------------------ subject context from claims


def test_subject_context_uses_configured_claim(monkeypatch):
    monkeypatch.setattr(
        deps,
        "_auth_config",
        {
            "auth": {
                "subject_types": [
                    {
                        "type": "group",
                        "priority": 1,
                        "auth_mechanisms": [{"type": "jwt", "entitlement": {"claim": "groups"}}],
                    }
                ]
            }
        },
    )
    ctx = extract_subject_context_from_claims({"sub": "u1", "groups": ["g1"]})
    assert ctx.type == "group"
    assert ctx.entitlement_claim == "groups"
    assert ctx.entitlement_values == ["g1"]


def test_subject_context_falls_back_to_sub(monkeypatch):
    monkeypatch.setattr(deps, "_auth_config", {})
    ctx = extract_subject_context_from_claims({"sub": "u1"})
    assert ctx.type == "user"
    assert ctx.entitlement_claim == "sub"
    assert ctx.entitlement_values == ["u1"]


# --------------------------------------------------------- require_authn


@pytest.mark.asyncio
async def test_require_authn_500_when_jwt_disabled(monkeypatch):
    monkeypatch.setattr(deps, "get_jwt_validator", lambda: DisabledJWTValidator())
    with pytest.raises(HTTPException) as exc:
        await require_authn(_request({"Authorization": "Bearer t"}))
    assert exc.value.status_code == 500


@pytest.mark.asyncio
async def test_require_authn_401_when_token_missing(monkeypatch):
    monkeypatch.setattr(deps, "get_jwt_validator", lambda: MagicMock())
    with pytest.raises(HTTPException) as exc:
        await require_authn(_request({}))
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_require_authn_401_when_validate_fails(monkeypatch):
    validator = MagicMock()
    validator.validate.side_effect = JWTValidationError("bad")
    monkeypatch.setattr(deps, "get_jwt_validator", lambda: validator)
    with pytest.raises(HTTPException) as exc:
        await require_authn(_request({"Authorization": "Bearer t"}))
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_require_authn_success_returns_subject(monkeypatch):
    validator = MagicMock()
    validator.validate.return_value = {"sub": "u1"}
    monkeypatch.setattr(deps, "get_jwt_validator", lambda: validator)
    monkeypatch.setattr(deps, "_auth_config", {})
    req = _request({"Authorization": "Bearer tok"})
    ctx = await require_authn(req)
    assert ctx.entitlement_values == ["u1"]
    assert req.state.bearer_token == "tok"


# ----------------------------------------------- authorization checker


@pytest.mark.asyncio
async def test_authorization_checker_allows(monkeypatch):
    client = MagicMock()
    client.evaluate = AsyncMock(return_value=Decision(decision=True))
    monkeypatch.setattr(deps, "get_authz_client", lambda: client)
    checker = AuthorizationChecker(action="rcareport:view", resource_type="rcareport")
    subject = SubjectContext(type="user", entitlementClaim="sub", entitlementValues=["u1"])
    result = await checker(
        _request({"Authorization": "Bearer t"}, body={"projectUid": "p"}), subject
    )
    assert result is subject


@pytest.mark.asyncio
async def test_authorization_checker_denies(monkeypatch):
    client = MagicMock()
    client.evaluate = AsyncMock(return_value=Decision(decision=False))
    monkeypatch.setattr(deps, "get_authz_client", lambda: client)
    checker = AuthorizationChecker(action="rcareport:view", resource_type="rcareport")
    subject = SubjectContext(type="user", entitlementClaim="sub", entitlementValues=["u1"])
    with pytest.raises(HTTPException) as exc:
        await checker(_request({"Authorization": "Bearer t"}, body={}), subject)
    assert exc.value.status_code == 403


@pytest.mark.asyncio
async def test_report_checker_extracts_project_from_path(monkeypatch):
    captured = {}

    async def fake_eval(request, token):
        captured["hierarchy"] = request.resource.hierarchy
        return Decision(decision=True)

    client = MagicMock()
    client.evaluate = AsyncMock(side_effect=fake_eval)
    monkeypatch.setattr(deps, "get_authz_client", lambda: client)
    checker = ReportAuthorizationChecker(action="rcareport:view", resource_type="rcareport")
    subject = SubjectContext(type="user", entitlementClaim="sub", entitlementValues=["u1"])
    await checker(
        _request({"Authorization": "Bearer t"}, path_params={"project_id": "proj-9"}), subject
    )
    assert captured["hierarchy"].project == "proj-9"


# --------------------------------------------------------- authz client


def _authz_client_with_response(response):
    client = AuthzClient(base_url="http://authz", timeout=5)
    fake = MagicMock()
    fake.post = AsyncMock(return_value=response)
    client._client = fake
    return client


def _response(status_code, json_data=None, text=""):
    resp = MagicMock()
    resp.status_code = status_code
    resp.json.return_value = json_data
    resp.text = text
    return resp


def _eval_request():
    return EvaluateRequest(
        subjectContext=SubjectContext(
            type="user", entitlementClaim="sub", entitlementValues=["u1"]
        ),
        resource=Resource(type="rcareport", hierarchy=ResourceHierarchy()),
        action="rcareport:view",
    )


@pytest.mark.asyncio
async def test_authz_evaluate_returns_decision_on_200():
    client = _authz_client_with_response(_response(200, [{"decision": True}]))
    decision = await client.evaluate(_eval_request(), "tok")
    assert decision.decision is True


@pytest.mark.asyncio
async def test_authz_evaluate_401_maps_to_http_401():
    client = _authz_client_with_response(_response(401))
    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request())
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_authz_evaluate_403_maps_to_http_403():
    client = _authz_client_with_response(_response(403))
    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request())
    assert exc.value.status_code == 403


@pytest.mark.asyncio
async def test_authz_evaluate_500_maps_to_503():
    client = _authz_client_with_response(_response(500, text="boom"))
    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request())
    assert exc.value.status_code == 503


@pytest.mark.asyncio
async def test_authz_evaluate_empty_decisions_maps_to_503():
    client = _authz_client_with_response(_response(200, []))
    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request())
    assert exc.value.status_code == 503


@pytest.mark.asyncio
async def test_authz_evaluate_connect_error_maps_to_503():
    import httpx

    client = AuthzClient(base_url="http://authz", timeout=5)
    fake = MagicMock()
    fake.post = AsyncMock(side_effect=httpx.ConnectError("down"))
    client._client = fake
    with pytest.raises(HTTPException) as exc:
        await client.evaluate(_eval_request())
    assert exc.value.status_code == 503


# --------------------------------------------------------------- jwt


def test_disabled_validator_returns_empty_claims():
    assert DisabledJWTValidator().validate("anything") == {}


def test_get_jwt_validator_disabled_without_jwks_url(monkeypatch):
    monkeypatch.setattr(jwt_module, "_jwt_validator", None)
    monkeypatch.setattr(jwt_module.settings, "jwt_jwks_url", "")
    assert isinstance(get_jwt_validator(), DisabledJWTValidator)


def test_get_jwt_validator_real_with_jwks_url(monkeypatch):
    monkeypatch.setattr(jwt_module, "_jwt_validator", None)
    monkeypatch.setattr(jwt_module.settings, "jwt_jwks_url", "https://idp/jwks")
    assert isinstance(get_jwt_validator(), JWTValidator)


def _validator_with_mocked_jwks(monkeypatch):
    v = JWTValidator(jwks_url="https://idp/jwks")
    signing = MagicMock()
    signing.key = "key"
    jwks_client = MagicMock()
    jwks_client.get_signing_key_from_jwt.return_value = signing
    monkeypatch.setattr(v, "_get_jwks_client", lambda: jwks_client)
    return v


def test_jwt_validate_success(monkeypatch):
    v = _validator_with_mocked_jwks(monkeypatch)
    monkeypatch.setattr(jwt_module.jwt, "decode", lambda *a, **k: {"sub": "u1"})
    assert v.validate("tok") == {"sub": "u1"}


def test_jwt_validate_expired_raises(monkeypatch):
    v = _validator_with_mocked_jwks(monkeypatch)

    def boom(*a, **k):
        raise pyjwt.ExpiredSignatureError()

    monkeypatch.setattr(jwt_module.jwt, "decode", boom)
    with pytest.raises(JWTValidationError, match="expired"):
        v.validate("tok")


def test_jwt_validate_jwks_fetch_error(monkeypatch):
    v = JWTValidator(jwks_url="https://idp/jwks")

    def boom():
        raise jwt_module.PyJWKClientError("no keys")

    monkeypatch.setattr(v, "_get_jwks_client", boom)
    with pytest.raises(JWTValidationError, match="Failed to fetch signing key"):
        v.validate("tok")
