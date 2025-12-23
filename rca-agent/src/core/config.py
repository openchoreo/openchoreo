# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Configuration settings for RCA agents."""

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Main settings for the RCA system."""

    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="allow",
    )

    rca_model_name: str = ""
    rca_llm_api_key: str = ""

    # URLs configurable via environment
    mcp_observability_url: str = "http://observer:8080/mcp"
    mcp_openchoreo_url: str = (
        "http://openchoreo-api.openchoreo-control-plane.svc.cluster.local:8080/mcp"
    )

    # Middleware flags
    debug: bool = False
    use_filesystem: bool = False

    # OpenSearch config
    opensearch_address: str = "https://opensearch:9200"
    opensearch_username: str = "admin"
    opensearch_password: str = "ThisIsTheOpenSearchPassword1"

    # OAuth2 Client Credentials
    oauth_token_url: str = ""
    oauth_client_id: str = ""
    oauth_client_secret: str = ""


settings = Settings()
