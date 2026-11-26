# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the text helpers shared by the check authoring cards."""

from src.agent.words import first_upper, lower_first


def test_first_upper():
    assert first_upper("approving reviews") == "Approving reviews"
    assert first_upper("") == ""


def test_lower_first_keeps_acronyms():
    assert lower_first("Approving reviews") == "approving reviews"
    assert lower_first("README file") == "README file"
