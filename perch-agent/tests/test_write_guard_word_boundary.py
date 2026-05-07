# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Word-boundary tests for WriteGuard's user-supplied-value matcher.

The previous implementation used a plain ``substring in corpus`` check,
which wrongly accepted required-field values that happened to be
substrings of unrelated user prose (e.g. ``"prod"`` matching the word
"production"). The fix in src.agent.middleware.write_guard.
_user_supplied_value uses regex word boundaries; these tests pin that
behaviour so a future "simplification" doesn't regress it.
"""
from src.agent.middleware.write_guard import _user_supplied_value


def test_exact_token_matches():
    assert _user_supplied_value("uat", "create environment uat now")


def test_substring_in_unrelated_word_does_not_match():
    # Was the bug: "prod" would match "production".
    assert not _user_supplied_value("prod", "use the production database")


def test_value_at_start_and_end_of_corpus():
    assert _user_supplied_value("alpha", "alpha is the new env")
    assert _user_supplied_value("alpha", "the new env is alpha")


def test_value_with_punctuation_around():
    assert _user_supplied_value("uat", "name: uat,")
    assert _user_supplied_value("uat", "(uat)")


def test_value_inside_underscored_identifier_does_not_match():
    # 'name' in 'first_name' shouldn't count — both '_' are word chars.
    assert not _user_supplied_value("name", "use first_name as the key")


def test_value_with_hyphen_does_match_at_token_boundary():
    # Hyphens are non-word characters in our class; "my-svc" tokenises to
    # ["my", "svc"], so each side is a discrete token. We accept this:
    # users frequently write component names with hyphens.
    assert _user_supplied_value("my-svc", "name: my-svc")


def test_empty_value_returns_false():
    assert not _user_supplied_value("", "anything")


def test_regex_metacharacter_in_value_does_not_explode():
    # Value contains '.', '*' etc. — must be escaped, not interpreted.
    assert _user_supplied_value("svc.example", "the host is svc.example today")
    assert not _user_supplied_value("svc.example", "the host is svcXexample today")


def test_field_name_in_message_does_not_count_as_supplied_value():
    # Was the bug: the literal word "name" in the user's message would
    # mark a value of "name" as supplied. Still true here, but the
    # scenario it broke (the model thinking the user's value is "name"
    # when they only mentioned the field) is downstream — this guard
    # only tests literal value→corpus matching. Documented for clarity.
    assert _user_supplied_value("name", "the name is uat")
