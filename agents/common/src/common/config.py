# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pydantic_settings import BaseSettings, SettingsConfigDict


class CommonSettings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="allow",
    )

    jwt_jwks_url: str = ""
    jwt_issuer: str = ""
    jwt_audience: str = ""
    jwt_jwks_refresh_interval: int = 3600
    jwt_insecure_allow_unverified: bool = False

    openchoreo_api_url: str = (
        "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080"
    )
    authz_timeout_seconds: int = 30
    auth_config_path: str = "auth-config.yaml"

    oauth_token_url: str = ""
    oauth_client_id: str = ""
    oauth_client_secret: str = ""
    oauth_scope: str = ""

    log_level: str = "INFO"
    openai_debug_logs: bool = False
    cors_allowed_origins: str = ""

    tls_insecure_skip_verify: bool = False
    jwks_url_tls_insecure_skip_verify: bool = False
    authz_tls_insecure_skip_verify: bool = False

    @property
    def authz_service_url(self) -> str:
        return self.openchoreo_api_url.rstrip("/")

    @property
    def jwks_verify_ssl(self) -> bool:
        return not (self.jwks_url_tls_insecure_skip_verify or self.tls_insecure_skip_verify)

    @property
    def authz_verify_ssl(self) -> bool:
        return not (self.authz_tls_insecure_skip_verify or self.tls_insecure_skip_verify)

    @property
    def oauth_verify_ssl(self) -> bool:
        return not self.tls_insecure_skip_verify
