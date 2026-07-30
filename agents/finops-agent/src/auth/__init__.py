# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.auth.authz_models import SubjectContext  # noqa: F401
from common.auth.jwt import (  # noqa: F401
    DisabledJWTValidator,
    JWTValidationError,
    JWTValidator,
)
from common.auth.oauth_client import OAuth2ClientCredentialsAuth  # noqa: F401
from common.auth.runtime import AuthRuntime, hierarchy_from_query
from src.config import settings

auth = AuthRuntime(settings, service_name="finops-agent")

get_jwt_validator = auth.get_jwt_validator
get_authz_client = auth.get_authz_client
get_oauth2_auth = auth.get_oauth2_auth
check_oauth2_connection = auth.check_oauth2_connection
require_authn = auth.require_authn

_report_hierarchy = hierarchy_from_query(project="project", namespace="namespace")
require_reports_authz = auth.checker(
    "finopsreport:view", "finopsreport", hierarchy=_report_hierarchy
)
require_reports_update_authz = auth.checker(
    "finopsreport:update", "finopsreport", hierarchy=_report_hierarchy
)
