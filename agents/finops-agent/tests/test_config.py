# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for ``Settings._validate_backend_config``."""

import pytest

from src.config import Settings


def test_sqlite_default_uri_is_filled_in():
    s = Settings(report_backend="sqlite", sql_backend_uri="")
    assert s.sql_backend_uri == "sqlite+aiosqlite:///data/finops_reports.db"


def test_postgresql_requires_uri():
    with pytest.raises(ValueError, match="requires: sql_backend_uri"):
        Settings(report_backend="postgresql", sql_backend_uri="")


def test_scheme_must_match_backend():
    with pytest.raises(ValueError, match="must match report_backend"):
        Settings(report_backend="postgresql", sql_backend_uri="sqlite+aiosqlite:///x.db")


def test_postgres_alias_is_normalized():
    # 'postgres' scheme is accepted as an alias for 'postgresql'.
    s = Settings(
        report_backend="postgresql",
        sql_backend_uri="postgres://user:pass@host:5432/db",
    )
    assert s.report_backend == "postgresql"


def test_postgresql_asyncpg_dialect_accepted():
    s = Settings(
        report_backend="postgresql",
        sql_backend_uri="postgresql+asyncpg://user:pass@host:5432/db",
    )
    assert s.report_backend == "postgresql"


def test_authz_service_url_strips_trailing_slash():
    s = Settings(openchoreo_api_url="http://api.example.com/")
    assert s.authz_service_url == "http://api.example.com"
