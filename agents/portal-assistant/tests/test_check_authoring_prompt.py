# Copyright 2026 The OpenChoreo Authors
# SPDX-License-Identifier: Apache-2.0

"""Tests for the CHECK AUTHORING branch of perch_prompt.j2 — new vs. editing."""

import pytest

from src.templates import render

_RETRIEVERS = [
    {
        "id": "githubRepositoryFactRetriever",
        "facts": [
            {
                "name": "githubVisibility",
                "type": "string",
                "description": "public, private or internal",
            },
        ],
    }
]
_RULE = '{"all":[{"fact":"githubVisibility","operator":"equal","value":"public"}]}'


def _render(**scope):
    return render(
        "prompts/perch_prompt.j2",
        {
            "read_tools": ["dry_run_check", "get_check"],
            "rca_tools_available": False,
            "scope": {"case_type": "check_authoring", **scope},
        },
    )


def test_facts_table_shows_the_kinds_each_retriever_collects_for():
    prompt = _render(
        available_fact_retrievers=[
            {**_RETRIEVERS[0], "kinds": ["component", "System"]},
            {"id": "anyRetriever", "facts": [{"name": "anyFact", "type": "string"}]},
        ],
    )
    assert "| collects facts for |" in prompt
    assert "| `githubRepositoryFactRetriever` | component, System | `githubVisibility` |" in prompt
    assert "| `anyRetriever` | any kind | `anyFact` |" in prompt


def test_prompt_refuses_a_kind_no_retriever_covers():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "**When no retriever covers the kind:**" in prompt
    assert "never use another kind's facts instead" in prompt
    assert "no fact fits or no retriever covers the kind" in prompt
    assert "**First decide: did the user ask for a check on an entity kind**" in prompt
    assert "Catalog kinds include Component, API, System, Domain" in prompt


def test_the_portal_offers_the_test_not_the_model():
    # The portal shows a component picker under every drafted check, so a
    # sentence from the model would say it twice.
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "offering to test it on a component" not in prompt
    assert "Don't offer to test it on a component" in prompt


def test_dry_run_target_follows_the_retrievers_kind():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "use a full ref (`<kind>:<namespace>/<name>`) as given" in prompt
    assert "otherwise ask for the entity's full ref instead of guessing" in prompt


def test_new_check_has_no_editing_section():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "### Editing an existing check" not in prompt
    assert "Response format (editing an existing check)" not in prompt
    assert "### Response format" in prompt
    assert "Suggested Maturity Tier:" in prompt


def test_editing_check_renders_current_check_and_edit_format():
    prompt = _render(
        available_fact_retrievers=_RETRIEVERS,
        selected_fact_ids=["githubRepositoryFactRetriever"],
        check_id="abc-123",
        check_name="Repo must be public",
        check_description="Fails if the repository is not public",
        current_rule=_RULE,
    )
    assert "### Editing an existing check" in prompt
    assert "`Repo must be public` (id `abc-123`)" in prompt
    assert "Fails if the repository is not public" in prompt
    assert _RULE in prompt
    assert "Response format (editing an existing check)" in prompt
    assert "Suggested related changes:" in prompt
    # The new-check form structure and its mandatory checklist must not
    # apply while editing — they would force Name/Category/Tier lines.
    assert "### Retriever, category, and maturity tier lines" not in prompt
    assert "BEFORE YOU REPLY — checklist (fact retrievers were forwarded" not in prompt


def test_editing_asks_for_confirmation_before_the_rule():
    """Editing is two steps: confirm the exact condition (and replace vs.
    add) in plain words first, then reply with the JSON + suggestions."""
    prompt = _render(check_id="abc-123", check_name="Repo must be public", current_rule=_RULE)
    assert "**Step 1 — confirm first.**" in prompt
    assert "#### Step 1 — confirm the change" in prompt
    assert "**Please confirm:**" in prompt
    assert "Replace or add" in prompt
    assert "#### Step 2 — the edited rule" in prompt


def test_editing_step2_checklist_names_the_real_description():
    """The Step 2 checklist quotes the check's own description so the
    model compares it with the edited rule and suggests a fix."""
    prompt = _render(
        check_id="abc-123",
        check_name="Repo must be public",
        check_description="Fails if the component's GitHub repository is not public",
        current_rule=_RULE,
    )
    assert "**BEFORE YOU REPLY in Step 2" in prompt
    assert 'the description ("Fails if the component\'s GitHub repository is not public")' in prompt
    assert "never look for, list or guess a component" in prompt
    # A text line before the json block keeps JSON-mode models from
    # emitting the rule object itself instead of {"message": ...}.
    assert "starts with the line `Edited rule:`" in prompt
    assert "never the rule object itself as the whole output" in prompt
    # Suggestions are always required, with an explicit "None" bullet.
    assert 'Then **always** add the "Suggested related changes" section' in prompt
    assert "- None — the name and description still match the new rule." in prompt


def test_new_check_has_no_confirmation_step():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "Step 1 — confirm" not in prompt


def test_editing_shows_the_saved_rule_and_never_loads_it():
    # An edit always carries its saved rule (ChatScope rejects one without):
    # the model never loads it with get_check.
    prompt = _render(check_id="abc-123", check_name="Repo must be public", current_rule=_RULE)
    assert f"- Current rule (its `conditions` object): `{_RULE}`" in prompt
    assert "call `get_check` once with id `abc-123`" not in prompt


def test_editing_step1_options_are_real_alternatives():
    """Step 1 options must be real alternatives, with a number example
    (inclusive vs. exclusive). The model says which values pass and fail."""
    prompt = _render(check_id="abc-123", check_name="Repo is active", current_rule=_RULE)
    assert "The server reads" not in prompt
    assert "<in plain words: what passes and what fails>" in prompt
    assert "**A rule describes when the check PASSES**" not in prompt
    assert 'Never offer "keep the current rule"' in prompt
    assert "1. **Proposed rule:** `<fact>` `lessThanInclusive` `60` — 60 or less" in prompt
    assert "2. **Proposed rule:** `<fact>` `lessThan` `60` — less than 60" in prompt


def test_only_an_answer_to_a_step1_question_skips_step1():
    prompt = _render(check_id="abc-123", check_name="Repo is active", current_rule=_RULE)
    assert "only when your previous message in this chat was a Step 1 question" in prompt
    assert "Any other message starts at Step 1 again" in prompt
    assert '("yes, but make it 70")' in prompt
    assert 'Never write the "Step 1" or "Step 2" headings in your reply.' in prompt


def test_grammar_names_the_list_operators():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "`equal` and `notEqual` compare one value, never a list." in prompt


def test_editing_step1_maps_user_words_to_one_reading():
    """Fix D: the user's words pick replace vs. add and the true/false value,
    so Step 1 asks one question instead of offering the opposite."""
    prompt = _render(check_id="abc-123", check_name="Branch deletion blocked", current_rule=_RULE)
    assert '"instead", "replace", "change it to", "swap" → **replace**' in prompt
    assert '"also", "too", "as well", "and", "plus" → **add**' in prompt
    assert "never offer the opposite of what they asked for" in prompt
    assert '"block" / "disable" / "prevent" → `equal false`' in prompt
    assert "also say whether the old condition is removed (replace) or kept (add)" in prompt


def test_check_authoring_examples_are_marked_as_illustrations():
    """Fix F: example fact names must not be taken as real facts, and the
    true/false rule works from the live table's meanings, not fixed names."""
    prompt = _render(check_id="abc-123", check_name="x", current_rule=_RULE)
    assert "**Examples in this section are illustrations only.**" in prompt
    assert "never the example's facts" in prompt
    rule_line = next(
        line for line in prompt.splitlines() if line.startswith("- **True/false facts:**")
    )
    assert "Available facts" in rule_line
    assert "`github" not in rule_line  # no hard-coded fact names in the rule itself


def test_prompt_names_only_the_retrievers_and_facts_it_was_given():
    """Retriever and fact names come from the forwarded table, so a backend
    with different retrievers never sees a name it doesn't have."""
    retrievers = [{"id": "acmeRetriever", "facts": [{"name": "acmeFact", "type": "boolean"}]}]
    new = _render(available_fact_retrievers=retrievers)
    editing = _render(
        available_fact_retrievers=retrievers, check_id="abc", check_name="x", current_rule="{}"
    )

    for prompt in (new, editing):
        assert "`acmeRetriever`" in prompt
        assert "`acmeFact`" in prompt
        assert "github" not in prompt.lower()


def test_new_check_flow_also_gets_the_illustration_note():
    assert "**Examples in this section are illustrations only.**" in _render(
        available_fact_retrievers=_RETRIEVERS
    )


def test_step1_current_rule_line_is_filled_in_by_the_server():
    prompt = render(
        "prompts/perch_prompt.j2",
        {
            "read_tools": [],
            "rca_tools_available": False,
            "scope": {"case_type": "check_authoring", "check_id": "abc", "current_rule": _RULE},
            "current_rule_summary": "`githubVisibility` `equal` `public`",
        },
    )
    assert (
        "- Current rule, exact summary (built from the saved rule): `githubVisibility` `equal` `public`"
        in prompt
    )
    assert "**Current rule:** `githubVisibility` `equal` `public` — <in plain words" in prompt
    assert "never describe a different fact, operator or value" in prompt


def test_edit_prompt_never_asks_for_how_to_pass():
    """Edits suggest only Name and Description; How to pass stays as saved,
    even when the launcher still forwards the check's text."""
    for extra in ({}, {"check_solution": "Turn on PR reviews"}):
        prompt = _render(check_id="abc", check_name="x", current_rule=_RULE, **extra)
        assert "How to pass" not in prompt
        assert "Solution:" not in prompt


def test_remove_means_delete_the_condition_not_flip_it():
    """Fix G: "remove the X requirement" deletes that condition; it never
    becomes `equal false`, and Step 1 asks one "Shall I remove …?" question."""
    prompt = _render(check_id="abc", check_name="x", current_rule=_RULE)
    assert "**Remove a condition**" in prompt
    assert "Never turn the removed condition into its opposite" in prompt
    assert "If removing it would leave no condition at all" in prompt


def test_step1_defaults_to_one_proposed_rule():
    """Fix M: one backtick-notation proposal by default; the server adds pass/fail."""
    prompt = _render(check_id="abc", check_name="x", current_rule=_RULE)
    assert "**By default make ONE proposal:**" in prompt
    assert (
        "**Proposed rule:** `<fact>` `<operator>` `<value>` — <in plain words: what passes and what fails>\n\nShall I change it to that?"
        in prompt
    )
    assert "do NOT write your own pass/fail explanation" not in prompt
    assert "write exactly the Proposed rule the user confirmed or picked" in prompt


def test_check_authoring_points_to_the_confirm_card_not_the_cli():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)

    assert "**Saving:**" in prompt
    assert "Save as draft" in prompt and "Save and publish" in prompt
    assert "never send the user to the OpenChoreo UI or CLI for a check" in prompt
    assert "paste in themselves" not in prompt


def test_other_cases_keep_the_plain_read_only_rule():
    prompt = render(
        "prompts/perch_prompt.j2",
        {"read_tools": [], "rca_tools_available": False, "scope": {"case_type": "build_failure"}},
    )

    assert "point them to the OpenChoreo UI or CLI" in prompt
    assert "Tech Insights checks are the exception" not in prompt
    assert "**Saving:**" not in prompt


def test_new_checks_are_validated_before_they_are_shown():
    prompt = _render(
        available_fact_retrievers=_RETRIEVERS,
        selected_fact_ids=["githubRepositoryFactRetriever"],
    )

    assert "### Validate every rule" in prompt
    assert "call `validate_check` with `factIds`" in prompt
    assert "`githubRepositoryFactRetriever` unless you chose another one" in prompt
    assert "After 3 attempts" in prompt
    assert "validate it (see above), and reply" in prompt
    assert "Did `validate_check` return `valid: true` for the rule in the reply" in prompt


def test_edits_validate_the_edited_rule_but_not_the_step1_question():
    prompt = _render(
        available_fact_retrievers=_RETRIEVERS,
        check_id="abc-123",
        check_name="Repo must be public",
        current_rule=_RULE,
    )

    assert "### Validate every rule" in prompt
    assert "validate the edited rule first" in prompt
    assert "Don't validate the Step 1 confirmation question" in prompt
    assert "Did `validate_check` return `valid: true` for the rule I show" in prompt


# ── P1: what validation and code now cover is not repeated in the prompt ─


def test_no_common_mistakes_list_and_a_short_grammar():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)

    assert "### Common mistakes to avoid" not in prompt
    assert "`validate_check` reports any shape, operator or fact problem with its path" in prompt
    # The one rule the validator can't catch stays.
    assert "never `notEqual` leaves stacked under `all`" in prompt


def test_one_worked_example_with_retriever_lines_only_when_forwarded():
    with_retrievers = _render(available_fact_retrievers=_RETRIEVERS)
    without = _render()

    for prompt in (with_retrievers, without):
        assert prompt.count('User: "Make sure the setting isn\'t turned on."') == 1
    assert "Suggested Maturity Tier: Bronze" in with_retrievers
    assert "Suggested Maturity Tier: Bronze" not in without


def test_no_own_pass_fail_rules_for_step1():
    prompt = _render(check_id="abc-123", check_name="Repo is active", current_rule=_RULE)

    assert "Say what happens at exactly N" not in prompt


# ── P2: one example per flow, short checklists ────────────────────────


def test_edit_flow_keeps_one_step1_example_and_no_step2_example():
    prompt = _render(check_id="abc-123", check_name="Repo is active", current_rule=_RULE)

    assert prompt.count("Example —") == 1
    assert "Example — bare number, two readings." in prompt
    # The rules the dropped examples showed are still stated.
    assert "Never turn the removed condition into its opposite" in prompt
    assert "never offer the opposite of what they asked for" in prompt
    assert '"at least N" / "N or more" → `greaterThanInclusive N`' in prompt


def test_step2_checklist_is_short():
    prompt = _render(check_id="abc-123", check_name="x", current_rule=_RULE)
    start = prompt.index("**BEFORE YOU REPLY in Step 2")
    checklist = prompt[start : prompt.index("If any box would be unchecked", start)]

    assert checklist.count("- [ ]") == 3


def test_no_stand_in_fact_when_none_fits():
    """'require a license file' with no license fact must not become a
    SECURITY.md check: the model says no fact fits and asks."""
    for prompt in (
        _render(available_fact_retrievers=_RETRIEVERS),
        _render(),
        _render(
            available_fact_retrievers=_RETRIEVERS,
            check_id="abc",
            check_name="x",
            current_rule=_RULE,
        ),
    ):
        assert "**When no fact fits:**" in prompt
        assert "Never use a different fact as a stand-in" in prompt


def test_no_fact_reply_is_not_forced_into_the_check_format():
    """Your test 9: the format offered "No relevant fact retriever available."
    and the checklist demanded category and tier lines, so the model replied
    with those instead of explaining and naming the closest facts."""
    prompt = _render(available_fact_retrievers=_RETRIEVERS)

    assert "No relevant fact retriever available" not in prompt
    assert "Checks you could write instead: `<fact>` (<a check on it, in plain words>)" in prompt
    assert "**Except when no fact fits or no retriever covers the kind**" in prompt
    assert "skip it when no fact fits" in prompt


def test_a_question_is_not_drafted_as_a_check():
    """ "Show me the full list of all facts" came back as a check named "All
    available facts" with an empty rule and a save card."""
    prompt = _render(available_fact_retrievers=_RETRIEVERS)

    assert "**Also except when the user asks a question instead of describing a check**" in prompt
    assert "Draft a check only when the user describes a rule they want enforced." in prompt


def test_a_facts_question_is_answered_at_once_within_the_card():
    for prompt in (_render(), _render(check_id="c1", current_rule=_RULE)):
        assert "When the user only asks which facts exist, they aren't picking" in prompt
        assert "don't ask whether they want a full or grouped list" in prompt
        assert "at most 5 sections of 8 lines" in prompt


def test_a_check_authoring_prompt_asks_for_the_reply_card():
    for prompt in (_render(), _render(check_id="c1", current_rule=_RULE)):
        assert "### Reply card" in prompt
        assert "also fill `card`" in prompt
        assert "A value is a few words; a sentence goes in `summary`." in prompt
        assert "show the likely items as `lines` and ask them to type the one they want" in prompt
        assert "- `choices` (empty in most replies; at most 2)" in prompt
        assert "Don't add them to every reply." in prompt
        assert "an easy next step that clearly fits what they asked" in prompt
        assert 'adds "Tell me something else" next to them, and only then' in prompt
        assert "End every card's `summary` with one short, friendly question" in prompt
        assert 'still uses the "No available fact measures …" wording' in prompt
        # 2026-11-19: an area's facts end by asking for the rule; a
        # requirement is drafted.
        assert "When the user asks to see all the facts for one area" in prompt
        assert (
            'end it with exactly: "Do you have a rule in mind? Tell me in your own words."'
            in prompt
        )
        assert "When the user gives a requirement instead, draft the check as usual." in prompt


def test_other_prompts_have_no_reply_card():
    prompt = render(
        "prompts/perch_prompt.j2",
        {"read_tools": [], "rca_tools_available": False, "scope": {"case_type": "build_failure"}},
    )

    assert "### Reply card" not in prompt
    assert "### How to write" not in prompt


def test_a_check_authoring_prompt_asks_for_a_teammates_words():
    # The mentor's feedback: some replies read as agent-generated.
    for prompt in (_render(), _render(check_id="c1", current_rule=_RULE)):
        assert "### How to write" in prompt
        assert "Write like a teammate on the security team" in prompt
        assert 'a sentence these instructions give "exactly" stays word for word' in prompt
        assert "Never use these words: craft, seamless, leverage" in prompt
        assert "Don't join clauses with a dash" in prompt


def test_a_dry_run_only_when_this_request_asks_for_one():
    # A component named for an earlier test isn't tested again on its own.
    new_check = _render(available_fact_retrievers=_RETRIEVERS)
    assert "does the user's latest message name an entity to test on" in new_check
    assert "A component named in an earlier message doesn't count" in new_check

    editing = _render(
        available_fact_retrievers=_RETRIEVERS,
        check_id="abc-123",
        current_rule=_RULE,
    )
    assert "a component named for an earlier change or test doesn't count" in editing
    assert "named a component in this conversation" not in editing


def test_an_added_condition_keeps_the_others_and_is_confirmed():
    prompt = _render(
        available_fact_retrievers=_RETRIEVERS,
        check_id="abc-123",
        current_rule=_RULE,
    )
    assert "keep **every** condition of the current rule exactly as it is" in prompt
    assert "Ask one question that names both" in prompt


def test_ideas_or_an_explanation_come_before_any_change():
    prompt = _render(
        available_fact_retrievers=_RETRIEVERS,
        check_id="abc-123",
        current_rule=_RULE,
    )
    assert "**Ideas or an explanation first.**" in prompt
    assert "don't propose a change yet" in prompt


def test_a_new_check_without_a_rule_in_mind_is_guided_step_by_step():
    new_check = _render(available_fact_retrievers=_RETRIEVERS)
    assert "**Guide a user who has no rule in mind yet, one step at a time.**" in new_check
    assert "don't draft a check and don't list the facts" in new_check
    assert "gets the draft at once — skip the guidance" in new_check
    # The areas they can cover, then several checks for the one picked, none singled out.
    assert '"Security areas you can check"' in new_check
    assert 'one section "Checks you could write"' in new_check
    assert "don't recommend, single out or explain only one" in new_check

    editing = _render(
        available_fact_retrievers=_RETRIEVERS,
        check_id="abc-123",
        current_rule=_RULE,
    )
    assert "Guide a user who has no rule in mind yet" not in editing


@pytest.mark.parametrize(
    ("experience", "words"),
    [
        ("beginner", "The user is new to checks."),
        ("experienced", "The user is familiar with checks."),
    ],
)
def test_the_prompt_suits_the_users_experience(experience, words):
    assert words in _render(experience=experience)


def test_no_experience_adds_no_level():
    prompt = _render()
    assert "The user is new to checks." not in prompt
    assert "The user is familiar with checks." not in prompt


def test_how_to_write_a_check_is_taught_not_answered_with_the_facts():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "**Teach how to write a check, don't list the facts.**" in prompt
    assert '"Show me how to write a check using available facts"' in prompt
    assert "never show the facts list for it" in prompt


def test_teaching_how_a_check_is_built_offers_no_suggestions():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "Leave `choices` empty: they answer that in their own words." in prompt
    assert '"What would you like to check? Tell me in your own words."' in prompt


def test_a_rule_has_only_the_conditions_the_user_asked_for():
    new_check = _render(available_fact_retrievers=_RETRIEVERS)
    edit = _render(available_fact_retrievers=_RETRIEVERS, check_id="abc-123", current_rule=_RULE)

    for prompt in (new_check, edit):
        assert "### Only what the user asked for" in prompt
        assert "Don't add a condition of your own, even a valid or sensible one" in prompt
        assert "no guard (such as checking that the repository could be read first)" in prompt


@pytest.mark.parametrize(
    ("scope", "line"),
    [
        ({}, "Suggested Fact Retriever: <the `id` of the `list_fact_schemas` schema"),
        (
            {"selected_fact_ids": ["r"]},
            "Fact Retriever: <the selected retriever id, verbatim> — or, if a different "
            "retriever from `list_fact_schemas`",
        ),
        (
            {"available_fact_retrievers": _RETRIEVERS},
            "Suggested Fact Retriever: <retriever id from the table above>",
        ),
    ],
)
def test_a_draft_always_names_its_retriever(scope, line):
    # Without the retriever line a drafted check gets no save card.
    prompt = _render(**scope)
    head = prompt[prompt.index("Description: <one-sentence description>") :]
    head = head[: head.index("Rule (json-rules-engine conditions)")]
    assert line in head
    assert "Suggested Category:" in head
    assert head.endswith("Suggested Maturity Tier: <one of Bronze, Silver, Gold>\n\n")


def test_the_check_tools_retry_limits_replace_the_core_budget():
    prompt = _render(available_fact_retrievers=_RETRIEVERS)
    assert "Never the same tool 3× per turn" in prompt
    assert (
        "For `validate_check` and `dry_run_check`, these limits (3 attempts each) replace the "
        '"never the same tool 3× per turn" budget above'
    ) in prompt
