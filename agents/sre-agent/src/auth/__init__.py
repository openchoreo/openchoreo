# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.auth.authz_models import SubjectContext  # noqa: F401
from common.auth.jwt import (  # noqa: F401
    DisabledJWTValidator,
    JWTValidationError,
    JWTValidator,
)
from common.auth.oauth_client import OAuth2ClientCredentialsAuth  # noqa: F401
from common.auth.runtime import (
    AuthRuntime,
    hierarchy_from_body,
    hierarchy_from_path,
)
from src.config import settings

auth = AuthRuntime(settings, service_name="rca-agent")

get_jwt_validator = auth.get_jwt_validator
get_authz_client = auth.get_authz_client
get_oauth2_auth = auth.get_oauth2_auth
check_oauth2_connection = auth.check_oauth2_connection
require_authn = auth.require_authn

require_chat_authz = auth.checker(
    "rcareport:view",
    "rcareport",
    hierarchy=hierarchy_from_body(project="projectUid", component="componentUid"),
)
require_reports_authz = auth.checker(
    "rcareport:view", "rcareport", hierarchy=hierarchy_from_path(project="project_id")
)
require_reports_update_authz = auth.checker(
    "rcareport:update", "rcareport", hierarchy=hierarchy_from_path(project="project_id")
)
