# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Small text helpers shared by the check authoring cards."""

from __future__ import annotations


def first_upper(text: str) -> str:
    """ "approving reviews" → "Approving reviews"."""
    return text[:1].upper() + text[1:]


def lower_first(text: str) -> str:
    """ "Approving reviews …" → "approving reviews …"; "README file" stays."""
    return text[:1].lower() + text[1:] if text[1:2].islower() else text
