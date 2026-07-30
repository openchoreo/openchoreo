# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the auth stack: bearer extraction, JWT, dependencies, authz client."""

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import jwt as pyjwt
import pytest
from fastapi import HTTPException

import common.auth.jwt as common_jwt
from common.auth import dependencies as common_deps
from common.auth.authz_client import AuthzClient
from common.auth.authz_errors import (
    AuthzForbidden,
    AuthzServiceUnavailable,
    AuthzUnauthorized,
)
from common.auth.authz_models import (
    Decision,
    EvaluateRequest,
    Resource,
    ResourceHierarchy,
    SubjectContext,
)
from common.auth.dependencies import extract_bearer_token
from common.auth.dependencies import extract_entitlements as _extract_entitlements
from src.auth import (
    DisabledJWTValidator,
    JWTValidationError,
    JWTValidator,
    auth,
    get_jwt_validator,
    require_authn,
    require_reports_authz,
)
from src.config import settings


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
        auth,
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
    ctx = auth.extract_subject_context({"sub": "u1", "groups": ["g1"]})
    assert ctx.type == "group"
    assert ctx.entitlement_claim == "groups"
    assert ctx.entitlement_values == ["g1"]


def test_subject_context_falls_back_to_sub(monkeypatch):
    monkeypatch.setattr(auth, "_auth_config", {})
    ctx = auth.extract_subject_context({"sub": "u1"})
    assert ctx.type == "user"
    assert ctx.entitlement_claim == "sub"
    assert ctx.entitlement_values == ["u1"]


# --------------------------------------------------------- require_authn


@pytest.mark.asyncio
async def test_require_authn_500_when_jwt_disabled(monkeypatch):
    monkeypatch.setattr(auth, "get_jwt_validator", lambda: DisabledJWTValidator())
    with pytest.raises(HTTPException) as exc:
        await require_authn(_request({"Authorization": "Bearer t"}))
    assert exc.value.status_code == 500


@pytest.mark.asyncio
async def test_require_authn_401_when_token_missing(monkeypatch):
    monkeypatch.setattr(auth, "get_jwt_validator", lambda: MagicMock())
    with pytest.raises(HTTPException) as exc:
        await require_authn(_request({}))
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_require_authn_401_when_validate_fails(monkeypatch):
    validator = MagicMock()
    validator.validate.side_effect = JWTValidationError("bad")
    monkeypatch.setattr(auth, "get_jwt_validator", lambda: validator)
    with pytest.raises(HTTPException) as exc:
        await require_authn(_request({"Authorization": "Bearer t"}))
    assert exc.value.status_code == 401


@pytest.mark.asyncio
async def test_require_authn_success_returns_subject(monkeypatch):
    validator = MagicMock()
    validator.validate = AsyncMock(return_value={"sub": "u1"})
    monkeypatch.setattr(auth, "get_jwt_validator", lambda: validator)
    monkeypatch.setattr(auth, "_auth_config", {})
    req = _request({"Authorization": "Bearer tok"})
    ctx = await require_authn(req)
    assert ctx.entitlement_values == ["u1"]
    assert req.state.bearer_token == "tok"


# ----------------------------------------------- authorization checker


@pytest.mark.asyncio
async def test_authorization_checker_allows(monkeypatch):
    client = MagicMock()
    client.evaluate = AsyncMock(return_value=Decision(decision=True))
    monkeypatch.setattr(auth, "get_authz_client", lambda: client)
    checker = auth.checker("rcareport:view", "rcareport")
    subject = SubjectContext(type="user", entitlementClaim="sub", entitlementValues=["u1"])
    result = await checker(
        _request({"Authorization": "Bearer t"}, body={"projectUid": "p"}), subject
    )
    assert result is subject


@pytest.mark.asyncio
async def test_authorization_checker_denies(monkeypatch):
    client = MagicMock()
    client.evaluate = AsyncMock(return_value=Decision(decision=False))
    monkeypatch.setattr(auth, "get_authz_client", lambda: client)
    checker = auth.checker("rcareport:view", "rcareport")
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
    monkeypatch.setattr(auth, "get_authz_client", lambda: client)
    checker = require_reports_authz
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
async def test_authz_evaluate_401_raises_unauthorized():
    client = _authz_client_with_response(_response(401))
    with pytest.raises(AuthzUnauthorized):
        await client.evaluate(_eval_request())


@pytest.mark.asyncio
async def test_authz_evaluate_403_raises_forbidden():
    client = _authz_client_with_response(_response(403))
    with pytest.raises(AuthzForbidden):
        await client.evaluate(_eval_request())


@pytest.mark.asyncio
async def test_authz_evaluate_500_raises_unavailable():
    client = _authz_client_with_response(_response(500, text="boom"))
    with pytest.raises(AuthzServiceUnavailable):
        await client.evaluate(_eval_request())


@pytest.mark.asyncio
async def test_authz_evaluate_empty_decisions_raises_unavailable():
    client = _authz_client_with_response(_response(200, []))
    with pytest.raises(AuthzServiceUnavailable):
        await client.evaluate(_eval_request())


@pytest.mark.asyncio
async def test_authz_evaluate_malformed_payload_raises_unavailable():
    # Decision.model_validate fails on a decision missing the required field.
    client = _authz_client_with_response(_response(200, [{"not_a_decision": True}]))
    with pytest.raises(AuthzServiceUnavailable):
        await client.evaluate(_eval_request())


@pytest.mark.asyncio
async def test_authz_evaluate_connect_error_raises_unavailable():
    import httpx

    client = AuthzClient(base_url="http://authz", timeout=5)
    fake = MagicMock()
    fake.post = AsyncMock(side_effect=httpx.ConnectError("down"))
    client._client = fake
    with pytest.raises(AuthzServiceUnavailable):
        await client.evaluate(_eval_request())


# --------------------------------------------------------------- jwt


async def test_disabled_validator_returns_empty_claims():
    assert await DisabledJWTValidator().validate("anything") == {}


def test_get_jwt_validator_disabled_without_jwks_url(monkeypatch):
    monkeypatch.setattr(auth, "_validator", None)
    monkeypatch.setattr(settings, "jwt_jwks_url", "")
    monkeypatch.setattr(settings, "jwt_insecure_allow_unverified", True)
    assert isinstance(get_jwt_validator(), DisabledJWTValidator)


def test_get_jwt_validator_fails_closed_without_jwks_url(monkeypatch):
    monkeypatch.setattr(auth, "_validator", None)
    monkeypatch.setattr(settings, "jwt_jwks_url", "")
    monkeypatch.setattr(settings, "jwt_insecure_allow_unverified", False)
    with pytest.raises(RuntimeError, match="JWT_JWKS_URL is required"):
        get_jwt_validator()


def test_get_jwt_validator_real_with_jwks_url(monkeypatch):
    monkeypatch.setattr(auth, "_validator", None)
    monkeypatch.setattr(settings, "jwt_jwks_url", "https://idp/jwks")
    monkeypatch.setattr(settings, "jwt_issuer", "https://idp")
    assert isinstance(get_jwt_validator(), JWTValidator)


def _validator_with_mocked_jwks(monkeypatch):
    v = JWTValidator(jwks_url="https://idp/jwks", issuer="https://idp")
    signing = MagicMock()
    signing.key = "key"
    jwks_client = MagicMock()
    jwks_client.get_signing_key_from_jwt.return_value = signing
    monkeypatch.setattr(v, "_jwks_client", jwks_client)
    return v


async def test_jwt_validate_success(monkeypatch):
    v = _validator_with_mocked_jwks(monkeypatch)
    monkeypatch.setattr(common_jwt.jwt, "decode", lambda *a, **k: {"sub": "u1"})
    assert await v.validate("tok") == {"sub": "u1"}


async def test_jwt_validate_expired_raises(monkeypatch):
    v = _validator_with_mocked_jwks(monkeypatch)

    def boom(*a, **k):
        raise pyjwt.ExpiredSignatureError()

    monkeypatch.setattr(common_jwt.jwt, "decode", boom)
    with pytest.raises(JWTValidationError, match="expired"):
        await v.validate("tok")


async def test_jwt_validate_jwks_fetch_error(monkeypatch):
    v = JWTValidator(jwks_url="https://idp/jwks", issuer="https://idp")
    jwks_client = MagicMock()
    jwks_client.get_signing_key_from_jwt.side_effect = common_jwt.PyJWKClientError("no keys")
    monkeypatch.setattr(v, "_jwks_client", jwks_client)
    with pytest.raises(JWTValidationError, match="Failed to fetch signing key"):
        await v.validate("tok")


def test_load_auth_config_rejects_non_mapping(tmp_path):
    p = tmp_path / "auth-config.yaml"
    p.write_text("- just\n- a\n- list\n")
    with pytest.raises(ValueError, match="must be a YAML mapping"):
        common_deps.load_auth_config(str(p))


def test_load_auth_config_rejects_malformed_subject_types(tmp_path):
    p = tmp_path / "auth-config.yaml"
    p.write_text("auth:\n  subject_types:\n    - not-a-mapping\n")
    with pytest.raises(ValueError, match="list of mappings"):
        common_deps.load_auth_config(str(p))


def test_hierarchy_from_path_keeps_falsy_values():
    from common.auth.runtime import hierarchy_from_path

    extract = hierarchy_from_path(project="project_id")
    req = _request({}, path_params={"project_id": 0})
    assert extract(req).project == "0"


@pytest.mark.asyncio
async def test_authz_evaluate_retries_transient_connect_errors(monkeypatch):
    import httpx

    from common.auth import authz_client as ac

    monkeypatch.setattr(ac, "_RETRY_BACKOFF_SECONDS", 0)
    client = AuthzClient(base_url="http://authz", timeout=5)
    ok = _response(200, [{"decision": True}])
    fake = MagicMock()
    fake.post = AsyncMock(side_effect=[httpx.ConnectError("down"), httpx.ConnectError("down"), ok])
    client._client = fake
    decision = await client.evaluate(_eval_request())
    assert decision.decision is True
    assert fake.post.await_count == 3


@pytest.mark.asyncio
async def test_authz_evaluate_gives_up_after_bounded_retries(monkeypatch):
    import httpx

    from common.auth import authz_client as ac

    monkeypatch.setattr(ac, "_RETRY_BACKOFF_SECONDS", 0)
    client = AuthzClient(base_url="http://authz", timeout=5)
    fake = MagicMock()
    fake.post = AsyncMock(side_effect=httpx.ConnectError("down"))
    client._client = fake
    with pytest.raises(AuthzServiceUnavailable):
        await client.evaluate(_eval_request())
    assert fake.post.await_count == 3
