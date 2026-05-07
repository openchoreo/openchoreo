# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import logging
import re
from typing import Any

import jiter

logger = logging.getLogger(__name__)


def _coerce_to_dict(result: Any) -> dict[str, Any] | None:
    """Normalise jiter's partial-parse output into the dict shape the
    parser expects.

    The model is contracted to emit a ChatResponse object (a JSON dict),
    but smaller models occasionally wrap it in a single-element list:
      [{"message": "...", "actions": []}]
    Drop-on-non-dict was the original behaviour and silently lost the
    streamed message until the final fallback. We now unwrap a
    single-element list whose sole element is a dict; anything else
    (multi-element list, scalar, etc.) is still treated as malformed
    but logged at DEBUG so a regression is visible.
    """
    if isinstance(result, dict):
        return result
    if isinstance(result, list) and len(result) == 1 and isinstance(result[0], dict):
        return result[0]
    if result is not None:
        logger.debug(
            "parse_partial_json: discarded non-dict result of type %s",
            type(result).__name__,
        )
    return None


def parse_partial_json(json_str: str) -> dict[str, Any] | None:
    try:
        result = jiter.from_json(json_str.encode(), partial_mode="trailing-strings")
        return _coerce_to_dict(result)
    except ValueError:
        json_str = re.sub(r"\\u[0-9a-fA-F]{0,3}$", "", json_str)
        json_str = re.sub(r"\\$", "", json_str)
        try:
            result = jiter.from_json(json_str.encode(), partial_mode="trailing-strings")
            return _coerce_to_dict(result)
        except ValueError:
            return None


class ChatResponseParser:
    def __init__(self) -> None:
        self._buffer = ""
        self._message = ""
        self._actions: list[Any] = []

    def push(self, chunk: str) -> str | None:
        self._buffer += chunk
        parsed = parse_partial_json(self._buffer)

        if parsed is None:
            return None

        if "actions" in parsed and isinstance(parsed["actions"], list):
            self._actions = parsed["actions"]

        new_message = parsed.get("message", "")
        if len(new_message) > len(self._message):
            delta = new_message[len(self._message) :]
            self._message = new_message
            return delta

        return None

    @property
    def message(self) -> str:
        return self._message

    @property
    def actions(self) -> list[Any]:
        return self._actions
