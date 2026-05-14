# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

from urllib.parse import urlparse

from pydantic import model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="allow",
    )

    llm_name: str = ""
    llm_api_key: str = ""

    observability_mcp_server_url: str = "http://observer:8080/mcp"
    opencost_mcp_server_url: str = (
        "http://opencost.openchoreo-observability-plane.svc.cluster.local:8081"
    )

    oauth_token_url: str = ""
    oauth_client_id: str = ""
    oauth_client_secret: str = ""
    oauth_scope: str = ""

    jwt_jwks_url: str = ""
    jwt_issuer: str = ""
    jwt_audience: str = ""
    jwt_jwks_refresh_interval: int = 3600
    authz_timeout_seconds: int = 30
    auth_config_path: str = "auth-config.yaml"
    openchoreo_api_url: str = "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080"

    @property
    def authz_service_url(self) -> str:
        return self.openchoreo_api_url.rstrip("/")

    report_backend: str = "sqlite"
    sql_backend_uri: str = ""

    finops_llm_max_tokens: int = 16384
    tool_result_max_chars: int = 8000

    max_concurrent_analyses: int = 5
    analysis_timeout_seconds: int = 600

    remediation_enabled: bool = False

    log_level: str = "INFO"
    tls_insecure_skip_verify: bool = False
    cors_allowed_origins: str = ""

    @model_validator(mode="after")
    def _validate_backend_config(self) -> Settings:
        if self.report_backend == "postgresql" and not self.sql_backend_uri:
            raise ValueError("report_backend='postgresql' requires: sql_backend_uri")
        if self.report_backend == "sqlite" and not self.sql_backend_uri:
            self.sql_backend_uri = "sqlite+aiosqlite:///data/finops_reports.db"
        if self.sql_backend_uri:
            scheme = urlparse(self.sql_backend_uri).scheme
            # Extract database type (before '+' for SQLAlchemy dialects like 'postgresql+asyncpg')
            db_type = scheme.split('+')[0] if '+' in scheme else scheme
            # Normalize common aliases
            normalized_scheme = "postgresql" if db_type == "postgres" else db_type
            if normalized_scheme != self.report_backend:
                raise ValueError(
                    f"sql_backend_uri scheme must match report_backend='{self.report_backend}'"
                )
        return self


settings = Settings()
