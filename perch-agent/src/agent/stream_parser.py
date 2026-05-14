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

**Delta-stream contract (current).** The orchestrator emits
``message_chunk`` ndjson events incrementally as the model produces
tokens — it does not buffer the whole turn before painting. Each
``_yield_chunks`` iteration calls ``push`` on this parser and then
``pop_delta``; the orchestrator yields a ``message_chunk`` for any
non-empty delta. The consumer contract is therefore strictly
*append* — the Backstage chat drawer concatenates every delta into
its in-progress streaming buffer until the terminal ``done`` event
clears that buffer and writes the final message to the timeline.

The three parser methods cooperate as follows:

  - ``push(chunk)``: append raw streamed text to the buffer, re-parse
    incrementally, and update ``_message`` when the partial parse
    surfaces a new (longer) ``message`` value. Normal forward
    progress — ``pop_delta`` afterwards returns the new suffix.
  - ``set_message(msg)``: wholesale replacement, used by the
    structured-response recovery path when the streamed value was a
    placeholder and the agent's final state has the real diagnosis.
    Resets the ``_emitted_len`` cursor to 0 so the *next* ``pop_delta``
    returns the **entire** new message rather than a meaningless
    suffix-of-a-different-string. The drawer's terminal ``done``
    handler then clears its streaming buffer and writes only
    ``done.message``, so a placeholder briefly visible mid-stream is
    replaced cleanly.
  - ``pop_delta()``: returns content appended since the previous call
    and advances ``_emitted_len``. Idempotent — a second call with no
    intervening push returns ``""``.
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
        # Length of the message the streaming loop has already emitted
        # as ``message_chunk`` events. Lets ``pop_delta`` return only
        # the *new* content since the last call so the wire stays a
        # true delta stream rather than re-sending the prefix each tick.
        self._emitted_len = 0

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
        no prose), or when a placeholder needs to be replaced with
        the model's final longer diagnosis.

        Resets the ``pop_delta`` cursor — the new message is not a
        suffix of the old one, so the next ``pop_delta`` must return
        the full new message (telling the consumer "start over"). The
        downstream drawer clears its streaming buffer on the terminal
        ``done`` event regardless, so the final timeline is correct.
        """
        self._message = msg
        self._emitted_len = 0

    def pop_delta(self) -> str:
        """Return content appended since the previous call.

        Called by the streaming loop after each ``push`` so it can
        forward only the *new* text as a ``message_chunk`` event
        rather than re-sending the prefix each tick.

        Normal case: each ``push`` grows the message — the delta is
        the new suffix. Reset case: when the message shrinks (e.g.
        ``set_message`` swaps a placeholder out for a shorter final
        message), the prefix can't be derived from the current state
        alone, so the delta wraps to the full new message. The
        consumer's contract is "append" — a reset like this will
        visually double the text in the drawer until the terminal
        ``done`` event clears the streaming buffer, which is the
        existing post-replace correction path.
        """
        cur_len = len(self._message)
        if cur_len >= self._emitted_len:
            delta = self._message[self._emitted_len:]
        else:
            delta = self._message
        self._emitted_len = cur_len
        return delta
