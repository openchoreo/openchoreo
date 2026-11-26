# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for reading a dry-run's result for the test card."""

import json

import pytest

from src.agent.dry_run_view import (
    DryRun,
    actual_lines,
    actual_words,
    entity_name,
    fact_results,
    missing_facts,
    needed_lines,
    read_dry_run,
    reason_words,
)

_RULE = json.dumps(
    {
        "all": [
            {"fact": "githubBranchProtected", "operator": "equal", "value": True},
            {
                "fact": "githubRequiredApprovingReviewCount",
                "operator": "greaterThanInclusive",
                "value": 2,
            },
        ]
    }
)
_ARGS = json.dumps(
    {
        "entityRef": "component:default/order-api",
        "check": {"name": "n", "description": "", "factIds": ["r"], "rule": {"conditions": _RULE}},
    }
)
_RESULT = {
    "status": "failed",
    "passed": False,
    "summary": "The check was evaluated successfully: …",
    "factValues": {"githubBranchProtected": True, "githubRequiredApprovingReviewCount": 1},
    "reasons": [],
}
_SCOPE = {
    "available_fact_retrievers": [
        {
            "id": "r",
            "facts": [
                {"name": "githubBranchProtected", "description": "Branch protected"},
            ],
        }
    ]
}


def test_a_result_is_read_with_the_component_and_rule_of_its_call():
    # langchain-mcp-adapters returns content blocks, not a plain string.
    content = [{"type": "text", "text": json.dumps(_RESULT)}]

    run = read_dry_run(_ARGS, content)

    assert run == DryRun(
        status="failed",
        entity_ref="component:default/order-api",
        fact_values={"githubBranchProtected": True, "githubRequiredApprovingReviewCount": 1},
        reasons=[],
        rule=_RULE,
        fact_ids=["r"],
    )


@pytest.mark.parametrize(
    "content",
    [
        "Error: No entity component:default/nope in the catalog.",
        json.dumps({"status": "invalid"}),
        None,
    ],
)
def test_a_call_without_an_answer_gives_none(content):
    assert read_dry_run(_ARGS, content) is None


def test_a_result_without_its_call_still_reads():
    run = read_dry_run(None, json.dumps(_RESULT))

    assert (run.status, run.entity_ref, run.rule) == ("failed", None, None)


@pytest.mark.parametrize(
    ("ref", "name"),
    [
        ("component:default/order-api", ("order-api", "Component")),
        ("order-api", ("order-api", None)),
        (None, ("the component", None)),
    ],
)
def test_entity_name(ref, name):
    assert entity_name(ref) == name


@pytest.mark.parametrize(
    ("value", "words"),
    [
        (None, "not collected"),
        (True, "Yes"),
        (False, "No"),
        (3, "3"),
        ([], "none"),
        (["codeql", "trivy"], "codeql, trivy"),
    ],
)
def test_actual_words(value, words):
    assert actual_words(value) == words


def test_what_the_rule_needs_and_what_the_component_has():
    run = read_dry_run(_ARGS, json.dumps(_RESULT))

    assert needed_lines(run, _SCOPE) == [
        ("Branch protected", "Yes"),
        ("Required approving review count", "2 or more"),
    ]
    assert actual_lines(run, _SCOPE) == [
        ("Branch protected", "Yes"),
        ("Required approving review count", "1"),
    ]


_REF = "component:default/order-api"


@pytest.mark.parametrize(
    ("code", "message", "words"),
    [
        (
            "missing-fact-values",
            f"githubRepositoryFactRetriever collected facts for {_REF} but didn't report "
            "githubBranchProtected, githubRequiredApprovingReviewCount.",
            "The facts collected for order-api have no value for branch protected, "
            "required approving review count.",
        ),
        (
            "no-facts",
            f"githubRepositoryFactRetriever has no facts for {_REF}. Either it hasn't run …",
            "No facts have been collected for order-api yet: the fact retriever hasn't run since it "
            "appeared, or something it needs, such as an annotation or credentials, is missing.",
        ),
        (
            "retriever-not-covering",
            f"githubRepositoryFactRetriever doesn't collect facts for {_REF}: the entity is an API.",
            "These facts aren't collected for order-api: the entity is an API.",
        ),
        (
            "entity-filtered",
            f'The check\'s "Applies to" filter excludes {_REF}: it has spec.type service.',
            'The check\'s "Applies to" filter leaves out order-api: it has spec.type service.',
        ),
        (
            "no-result",
            f"The check produced no result for {_REF}.",
            "The check gave no result for order-api.",
        ),
        ("something-new", "A reason this code doesn't know.", "A reason this code doesn't know."),
    ],
)
def test_each_reason_reads_in_plain_words(code, message, words):
    run = DryRun(status="not-applicable", entity_ref=_REF, reasons=[(code, message)])

    assert reason_words(run, _SCOPE) == [words]


def test_facts_that_were_not_reported_are_listed():
    run = DryRun(
        status="not-applicable",
        entity_ref=_REF,
        reasons=[
            (
                "missing-fact-values",
                f"r collected facts for {_REF} but didn't report githubRequiredApprovingReviewCount.",
            )
        ],
    )

    assert missing_facts(run) == ["githubRequiredApprovingReviewCount"]
    assert actual_lines(run, _SCOPE) == [("Required approving review count", "not reported")]


def _marks(rule, values):
    run = DryRun(status="passed", fact_values=values, rule=json.dumps(rule))
    return [follows for _label, _value, follows, _wants in fact_results(run, {})]


def test_a_fact_is_marked_by_each_condition_the_rule_needs():
    assert _marks(
        json.loads(_RULE),
        {
            "githubBranchProtected": True,
            "githubRequiredApprovingReviewCount": 1,
        },
    ) == [True, False]


@pytest.mark.parametrize(
    "rule",
    [
        # private OR internal: "private" fails one choice, yet the check passes.
        {
            "any": [
                {"fact": "githubVisibility", "operator": "equal", "value": "private"},
                {"fact": "githubVisibility", "operator": "equal", "value": "internal"},
            ]
        },
        # not (public AND internal) is "not public OR not internal".
        {
            "not": {
                "all": [
                    {"fact": "githubVisibility", "operator": "equal", "value": "public"},
                    {"fact": "githubVisibility", "operator": "equal", "value": "internal"},
                ]
            }
        },
    ],
)
def test_a_fact_in_one_of_several_choices_gets_no_mark(rule):
    assert _marks(rule, {"githubVisibility": "private"}) == [None]


def test_an_any_of_one_condition_still_marks_it():
    rule = {"any": [{"fact": "githubVisibility", "operator": "equal", "value": "public"}]}
    assert _marks(rule, {"githubVisibility": "private"}) == [False]


def test_not_any_needs_every_condition_turned_around():
    rule = {
        "not": {
            "any": [
                {"fact": "githubVisibility", "operator": "equal", "value": "public"},
                {"fact": "githubArchived", "operator": "equal", "value": True},
            ]
        }
    }
    assert _marks(rule, {"githubVisibility": "private", "githubArchived": True}) == [True, False]


def test_a_result_keeps_the_retrievers_its_call_ran_with():
    args = {"entityRef": "component:default/a", "check": {"factIds": ["r1", "r2"], "rule": {}}}
    run = read_dry_run(args, {"status": "passed"})

    assert run.fact_ids == ["r1", "r2"]
    # Missing or malformed: unknown, not an empty list.
    assert read_dry_run({"check": {}}, {"status": "passed"}).fact_ids is None
    assert read_dry_run({"check": {"factIds": "r1"}}, {"status": "passed"}).fact_ids is None
    assert read_dry_run({"check": {"factIds": [1]}}, {"status": "passed"}).fact_ids is None


def test_a_failed_required_condition_keeps_its_mark_beside_optional_ones():
    # CodeRabbit: 1 review fails "at least 2", which the rule needs, however the
    # either-or conditions on the same fact read.
    reviews = "githubRequiredApprovingReviewCount"
    rule = {
        "all": [
            {"fact": reviews, "operator": "greaterThanInclusive", "value": 2},
            {
                "any": [
                    {"fact": reviews, "operator": "equal", "value": 0},
                    {"fact": reviews, "operator": "equal", "value": 1},
                ]
            },
        ]
    }

    assert _marks(rule, {reviews: 1}) == [False]
    # Met by the required condition, but none of its choices: the rule fails
    # for this fact, so no pass mark.
    assert _marks(rule, {reviews: 5}) == [None]


def test_a_pass_mark_needs_one_of_the_facts_choices_met_too():
    # CodeRabbit: reviews >= 2 AND (reviews == 2 OR reviews == 3).
    reviews = "githubRequiredApprovingReviewCount"
    rule = {
        "all": [
            {"fact": reviews, "operator": "greaterThanInclusive", "value": 2},
            {
                "any": [
                    {"fact": reviews, "operator": "equal", "value": 2},
                    {"fact": reviews, "operator": "equal", "value": 3},
                ]
            },
        ]
    }

    assert _marks(rule, {reviews: 1}) == [False]
    assert _marks(rule, {reviews: 2}) == [True]
    assert _marks(rule, {reviews: 5}) == [None]


def test_a_fact_only_in_optional_conditions_stays_unmarked_whatever_they_read():
    rule = {
        "any": [
            {"fact": "githubVisibility", "operator": "equal", "value": "private"},
            {"fact": "githubVisibility", "operator": "equal", "value": "internal"},
        ]
    }

    assert _marks(rule, {"githubVisibility": "public"}) == [None]
    assert _marks(rule, {"githubVisibility": "private"}) == [None]
