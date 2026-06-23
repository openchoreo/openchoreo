# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for time-range validation and OpenChoreo scope resolution."""

from unittest.mock import AsyncMock, patch

import pytest

from src.helpers import (
    resolve_component_scope,
    resolve_project_scope,
    validate_time_range,
)


def test_validate_time_range_normalizes_z_suffix():
    start, end = validate_time_range("2026-06-01T00:00:00Z", "2026-06-02T00:00:00Z")
    assert start == "2026-06-01T00:00:00+00:00"
    assert end == "2026-06-02T00:00:00+00:00"


def test_validate_time_range_rejects_start_after_end():
    with pytest.raises(ValueError, match="startTime must be before endTime"):
        validate_time_range("2026-06-02T00:00:00Z", "2026-06-01T00:00:00Z")


@pytest.mark.asyncio
async def test_resolve_component_scope_populates_all_uids():
    get_mock = AsyncMock(
        side_effect=[
            {"metadata": {"uid": "proj-uid"}},
            {"metadata": {"uid": "comp-uid"}},
            {"metadata": {"uid": "env-uid"}},
        ]
    )
    with (
        patch("src.helpers.get", get_mock),
        patch("src.helpers.get_oauth2_auth", return_value=object()),
    ):
        scope = await resolve_component_scope(
            namespace="ns", project="p", component="c", environment="dev"
        )

    assert scope.project_uid == "proj-uid"
    assert scope.component_uid == "comp-uid"
    assert scope.environment_uid == "env-uid"
    assert scope.namespace == "ns"
    assert scope.component == "c"


@pytest.mark.asyncio
async def test_resolve_project_scope_omits_component():
    get_mock = AsyncMock(
        side_effect=[
            {"metadata": {"uid": "proj-uid"}},
            {"metadata": {"uid": "env-uid"}},
        ]
    )
    with (
        patch("src.helpers.get", get_mock),
        patch("src.helpers.get_oauth2_auth", return_value=object()),
    ):
        scope = await resolve_project_scope(namespace="ns", project="p", environment="dev")

    assert scope.project_uid == "proj-uid"
    assert scope.environment_uid == "env-uid"
    assert scope.component is None
    assert scope.component_uid is None
