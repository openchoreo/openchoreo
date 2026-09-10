# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

from common.auth.runtime import AuthRuntime, hierarchy_from_query
from src.config import settings

auth = AuthRuntime(settings, service_name="finops-agent")

get_jwt_validator = auth.get_jwt_validator
get_oauth2_auth = auth.get_oauth2_auth
check_oauth2_connection = auth.check_oauth2_connection
require_authn = auth.require_authn

require_reports_authz = auth.checker(
    "finopsreport:view",
    "finopsreport",
    hierarchy=hierarchy_from_query(project="project", namespace="namespace"),
)
