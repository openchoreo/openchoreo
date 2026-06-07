# Copyright 2025 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

import re
from typing import Any

import jiter


def parse_partial_json(json_str: str) -> dict[str, Any] | None:
    try:
        result = jiter.from_json(json_str.encode(), partial_mode="trailing-strings")
        return result if isinstance(result, dict) else None
    except ValueError:
        json_str = re.sub(r"\\u[0-9a-fA-F]{0,3}$", "", json_str)
        json_str = re.sub(r"\\$", "", json_str)
        try:
            result = jiter.from_json(json_str.encode(), partial_mode="trailing-strings")
            return result if isinstance(result, dict) else None
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

        if "actions" in parsed:
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
