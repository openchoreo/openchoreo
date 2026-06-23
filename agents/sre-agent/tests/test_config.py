# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ``Settings._validate_backend_config`` and computed URL properties."""

import pytest

from src.config import Settings


def test_sqlite_default_uri_is_filled_in():
    s = Settings(report_backend="sqlite", sql_backend_uri="")
    assert s.sql_backend_uri == "sqlite+aiosqlite:///data/rca_reports.db"


def test_postgresql_requires_uri():
    with pytest.raises(ValueError, match="requires: sql_backend_uri"):
        Settings(report_backend="postgresql", sql_backend_uri="")


def test_scheme_must_match_backend():
    with pytest.raises(ValueError, match="must match report_backend"):
        Settings(report_backend="postgresql", sql_backend_uri="sqlite+aiosqlite:///x.db")


def test_postgresql_asyncpg_dialect_accepted():
    s = Settings(
        report_backend="postgresql",
        sql_backend_uri="postgresql+asyncpg://user:pass@host:5432/db",
    )
    assert s.report_backend == "postgresql"


def test_bare_postgres_scheme_is_rejected():
    # Unlike finops-agent, sre-agent does not normalize the ``postgres://`` alias:
    # the URI must literally start with the ``report_backend`` value.
    with pytest.raises(ValueError, match="must match report_backend"):
        Settings(
            report_backend="postgresql",
            sql_backend_uri="postgres://user:pass@host:5432/db",
        )


def test_observer_mcp_url_appends_mcp_and_strips_slash():
    s = Settings(observer_api_url="http://observer:8080/")
    assert s.observer_mcp_url == "http://observer:8080/mcp"


def test_openchoreo_mcp_url_appends_mcp_and_strips_slash():
    s = Settings(openchoreo_api_url="http://api.example.com/")
    assert s.openchoreo_mcp_url == "http://api.example.com/mcp"


def test_authz_service_url_strips_trailing_slash():
    s = Settings(openchoreo_api_url="http://api.example.com/")
    assert s.authz_service_url == "http://api.example.com"
