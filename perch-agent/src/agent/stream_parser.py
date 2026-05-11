# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Extract the ``message`` field from a streamed ChatResponse payload.

The agent emits ``ChatResponse`` args as ``{"message": "..."}``. We
accumulate chunks into a buffer and re-parse with jiter on each push —
``partial_mode="trailing-strings"`` tolerates incomplete JSON and a
half-closed string, so ``message`` can be read out as soon as the
model has written enough of it. jiter handles all JSON escapes
(``\\n``, ``\\"``, ``\\uXXXX``) correctly, which is why we no longer
maintain a hand-rolled escape table.

The orchestrator does not stream ``message_chunk`` deltas — every
chunk is buffered until the end of the turn (see the
``_yield_chunks`` docstring for why). The parser therefore only needs
to expose the *latest* message value, not per-chunk deltas.
"""

from __future__ import annotations

import logging

import jiter

logger = logging.getLogger(__name__)

# Cap accumulated stream bytes to bound memory under malformed / malicious
# input. A legitimate ChatResponse message is a few KB; the agent may stream
# in many chunks but the JSON envelope stays compact. 1 MiB is far above
# real traffic and far below a DoS-relevant footprint.
MAX_BUFFER_SIZE = 1 * 1024 * 1024


class StreamBufferOverflowError(Exception):
    """Raised when ``ChatResponseParser._buf`` exceeds ``MAX_BUFFER_SIZE``.

    Surfaces through the orchestrator's stream-error path so the user sees
    a normalized error + terminal done event rather than the process
    accumulating bytes until OOM.
    """


def _coerce_to_dict(parsed: object) -> dict | None:
    """Normalise jiter's partial-parse output into the dict shape we expect.

    The model is contracted to emit a ChatResponse object (a JSON
    dict), but smaller models occasionally wrap it in a single-element
    list: ``[{"message": "..."}]``. Unwrap that; everything else is
    treated as malformed.
    """
    if isinstance(parsed, dict):
        return parsed
    if isinstance(parsed, list) and len(parsed) == 1 and isinstance(parsed[0], dict):
        return parsed[0]
    return None


class ChatResponseParser:
    def __init__(self) -> None:
        self._buf = ""
        self._message = ""

    def push(self, chunk: str) -> None:
        if not chunk:
            return
        self._buf += chunk
        if len(self._buf) > MAX_BUFFER_SIZE:
            logger.warning(
                "ChatResponseParser buffer exceeded %d bytes; aborting stream",
                MAX_BUFFER_SIZE,
            )
            raise StreamBufferOverflowError(
                f"stream buffer exceeded {MAX_BUFFER_SIZE} bytes",
            )
        try:
            parsed = jiter.from_json(
                self._buf.encode(), partial_mode="trailing-strings",
            )
        except ValueError as e:
            # jiter raises plain ValueError for malformed JSON — it doesn't
            # expose a specific JSONDecodeError class. Partial parses fail
            # here on every incomplete chunk (the streaming protocol's hot
            # path), so log at DEBUG only: noisy in normal operation, but
            # surfaces context when something unexpected is going on.
            logger.debug(
                "jiter.from_json failed at buf_len=%d: %s; buf_prefix=%r",
                len(self._buf), e, self._buf[:200],
            )
            return
        as_dict = _coerce_to_dict(parsed)
        if as_dict is None:
            return
        msg = as_dict.get("message")
        if isinstance(msg, str):
            self._message = msg

    @property
    def message(self) -> str:
        return self._message

    def set_message(self, msg: str) -> None:
        """Overwrite the parsed message.

        Used by the structured-response recovery path when the
        streaming pass didn't surface a message via ``push`` (rare —
        usually only when the model emits a terminal ChatResponse with
        no prose).
        """
        self._message = msg
