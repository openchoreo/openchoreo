# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the error-string scrubber in ToolErrorHandlerMiddleware.

Tool errors get fed back into the LLM context, which then surfaces them
to the user. We strip obvious secret patterns and bound length.
"""
from src.agent.middleware.tool_error_handler import _MAX_ERROR_CHARS, _scrub


def test_redacts_bearer_token():
    out = _scrub("upstream replied 401 Unauthorized: bearer eyJhbGciOiJIUzI1NiIsInR5c")
    assert "[REDACTED]" in out
    assert "eyJhbGc" not in out


def test_redacts_authorization_header_kv():
    out = _scrub("Authorization: Bearer abc.def.ghi")
    assert "[REDACTED]" in out
    assert "abc.def.ghi" not in out


def test_redacts_env_style_secret_assignments():
    for key in ("KEY", "TOKEN", "SECRET", "PASSWORD"):
        out = _scrub(f"failed: {key}=hunter2 expired")
        assert "hunter2" not in out
        assert "[REDACTED]" in out


def test_does_not_redact_unrelated_text():
    out = _scrub("ImagePullBackOff: registry returned 404")
    assert out == "ImagePullBackOff: registry returned 404"


def test_truncates_oversize_input():
    long_input = "x" * (_MAX_ERROR_CHARS + 500)
    out = _scrub(long_input)
    assert len(out) <= _MAX_ERROR_CHARS + 50  # truncation suffix overhead
    assert "truncated" in out


def test_redaction_and_truncation_compose():
    payload = "Bearer " + "y" * 2000 + " trailing context"
    out = _scrub(payload)
    assert "[REDACTED]" in out
    # The truncation marker is appended after redaction, so the final
    # length still fits the budget.
    assert len(out) <= _MAX_ERROR_CHARS + 50


# ── Internal-endpoint redaction ──────────────────────────────────────────


def test_redacts_rfc1918_private_ips_with_optional_port():
    for sample in (
        "connect to 10.0.0.5:8080 failed",
        "dial 192.168.1.42 timed out",
        "172.16.0.7:443 closed",
        "172.31.255.1: closed",  # bare colon after IP is just a delimiter
        "host 172.20.5.5 unreachable",
    ):
        out = _scrub(sample)
        assert "[REDACTED]" in out, sample
        for needle in ("10.0.0.5", "192.168.1.42", "172.16.0.7", "172.31.255.1", "172.20.5.5"):
            assert needle not in out


def test_does_not_redact_non_private_ips():
    # 8.8.8.8 / 172.15 (just outside 172.16-31) / 192.169 must NOT match.
    out = _scrub("upstream 8.8.8.8:53 returned NXDOMAIN")
    assert "8.8.8.8" in out
    out = _scrub("172.15.0.1 is public")
    assert "172.15.0.1" in out
    out = _scrub("192.169.0.1 is also public")
    assert "192.169.0.1" in out


def test_redacts_localhost_and_loopback():
    out = _scrub("dialed 127.0.0.1:9090 and localhost:8080")
    assert "[REDACTED]" in out
    assert "127.0.0.1" not in out
    assert "localhost" not in out


def test_redacts_k8s_service_hostnames():
    out = _scrub("backend openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080 timed out")
    assert "[REDACTED]" in out
    assert "openchoreo-api" not in out
    assert ".svc" not in out


def test_redacts_internal_url_with_svc():
    out = _scrub(
        "GET http://observer.openchoreo-observability-plane.svc.cluster.local:8080/api returned 500"
    )
    assert "[REDACTED]" in out
    assert "observer" not in out


def test_does_not_redact_benign_colon_number_text():
    # The finding's bare ":\d+" suggestion would mangle these. Verify our
    # port-as-suffix-of-host-only rule leaves them untouched.
    for sample in (
        "error 500: not found",
        "timestamp 12:34:56",
        "(line 5:10) syntax error",
        "exit code 137",
        "completed in 250ms",
    ):
        assert _scrub(sample) == sample, sample


def test_redacts_multiple_endpoints_in_one_message():
    msg = (
        "stack: at 10.0.0.5:8080 -> api.openchoreo-control-plane.svc.cluster.local:8080 "
        "-> localhost:9090 -> 192.168.1.10"
    )
    out = _scrub(msg)
    for needle in (
        "10.0.0.5",
        "openchoreo-control-plane.svc",
        "localhost",
        "192.168.1.10",
    ):
        assert needle not in out, needle
    assert "[REDACTED]" in out
