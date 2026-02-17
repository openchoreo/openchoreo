# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="allow",
    )

    rca_model_name: str = ""
    rca_llm_api_key: str = ""

    # URLs configurable via environment
    observer_mcp_url: str = "http://observer:8080/mcp"
    control_plane_url: str = "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080"

    @property
    def openchoreo_mcp_url(self) -> str:
        return f"{self.control_plane_url.rstrip('/')}/mcp"

    # Logging
    log_level: str = "INFO"
    openai_debug_logs: bool = False

    # OpenSearch config
    opensearch_address: str = "https://opensearch:9200"
    opensearch_username: str = "admin"
    opensearch_password: str = "ThisIsTheOpenSearchPassword1"

    # OAuth2 Client Credentials
    oauth_token_url: str = ""
    oauth_client_id: str = ""
    oauth_client_secret: str = ""

    # Analysis concurrency and timeout settings
    max_concurrent_analyses: int = 5
    analysis_timeout_seconds: int = 1200

    # Skip TLS certificate verification (for self-signed certificates)
    tls_insecure_skip_verify: bool = False

    # JWT Authentication settings
    jwt_disabled: bool = False
    jwt_jwks_url: str = ""
    jwt_issuer: str = ""  # Optional: validate issuer claim
    jwt_audience: str = ""  # Optional: validate audience claim
    jwt_jwks_refresh_interval: int = 3600  # seconds (1 hour)

    # Authorization settings
    authz_timeout_seconds: int = 30

    @property
    def authz_service_url(self) -> str:
        return self.control_plane_url.rstrip("/")


settings = Settings()
