#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("external_compatibility_scan.py")
SPEC = importlib.util.spec_from_file_location("external_compatibility_scan", SCRIPT_PATH)
assert SPEC is not None
scanner = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = scanner
SPEC.loader.exec_module(scanner)


def finding(finding_id: str = "finding-1") -> dict[str, object]:
    return {
        "id": finding_id,
        "source_id": "source-1",
        "source_name": "Source 1",
        "severity": "high",
        "owner": "platform",
        "affected_files": [".github/external-compatibility-sources.json"],
        "action": "Review.",
        "title": "Deprecated API",
        "url": "https://example.com/deprecation",
        "summary": "A deprecated API was found.",
        "observed_at": "2026-06-23T00:00:00Z",
        "kind": "feed",
    }


class ExternalCompatibilityScanTests(unittest.TestCase):
    def test_validate_accepts_page_terms_with_match_terms(self) -> None:
        config = {
            "sources": [
                {
                    "id": "openai",
                    "name": "OpenAI",
                    "type": "page_terms",
                    "url": "https://example.com",
                    "affected_files": ["agents"],
                    "action": "Review.",
                    "match_terms": ["gpt-5"],
                }
            ]
        }

        self.assertEqual(scanner.validate_config(config), [])

    def test_validate_rejects_page_terms_without_match_terms(self) -> None:
        config = {
            "sources": [
                {
                    "id": "openai",
                    "name": "OpenAI",
                    "type": "page_terms",
                    "url": "https://example.com",
                    "affected_files": ["agents"],
                    "action": "Review.",
                }
            ]
        }

        self.assertIn("openai: page_terms sources require match_terms", scanner.validate_config(config))

    def test_validate_rejects_non_list_required_terms(self) -> None:
        config = {
            "sources": [
                {
                    "id": "feed",
                    "name": "Feed",
                    "type": "feed",
                    "url": "https://example.com/feed.xml",
                    "affected_files": ["agents"],
                    "action": "Review.",
                    "required_terms": "webhook",
                }
            ]
        }

        self.assertIn("feed: required_terms must be a list", scanner.validate_config(config))

    def test_validate_rejects_non_integer_required_terms_context_chars(self) -> None:
        config = {
            "sources": [
                {
                    "id": "feed",
                    "name": "Feed",
                    "type": "feed",
                    "url": "https://example.com/feed.xml",
                    "affected_files": ["agents"],
                    "action": "Review.",
                    "required_terms": ["webhook"],
                    "required_terms_context_chars": "near",
                }
            ]
        }

        self.assertIn("feed: required_terms_context_chars must be an integer", scanner.validate_config(config))

    def test_feed_http_error_is_not_treated_as_empty_feed(self) -> None:
        original_fetch = scanner.fetch_url
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 404, {}, "not found")
        try:
            with self.assertRaisesRegex(ValueError, "HTTP 404"):
                scanner.scan_feed(
                    {"defaults": {"keywords": ["deprecated"]}},
                    {
                        "id": "feed",
                        "name": "Feed",
                        "type": "feed",
                        "url": "https://example.com/feed.xml",
                        "affected_files": ["x"],
                        "action": "Review.",
                    },
                    1,
                )
        finally:
            scanner.fetch_url = original_fetch

    def test_malformed_feed_is_a_source_error(self) -> None:
        original_fetch = scanner.fetch_url
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 200, {}, "not xml")
        try:
            with self.assertRaisesRegex(ValueError, "feed XML parse failed"):
                scanner.scan_feed(
                    {"defaults": {"keywords": ["deprecated"]}},
                    {
                        "id": "feed",
                        "name": "Feed",
                        "type": "feed",
                        "url": "https://example.com/feed.xml",
                        "affected_files": ["x"],
                        "action": "Review.",
                    },
                    1,
                )
        finally:
            scanner.fetch_url = original_fetch

    def test_pending_findings_are_retried_until_marked_notified(self) -> None:
        state: dict[str, object] = {"initialized": True, "seen": {}}
        first = scanner.update_state_and_filter(
            state,
            [finding()],
            first_run=False,
            notify_on_first_run=False,
        )
        second = scanner.update_state_and_filter(
            state,
            [finding()],
            first_run=False,
            notify_on_first_run=False,
        )

        self.assertEqual([item["id"] for item in first], ["finding-1"])
        self.assertEqual([item["id"] for item in second], ["finding-1"])
        self.assertIn("finding-1", state["pending"])
        self.assertNotIn("finding-1", state["notified"])

        self.assertEqual(scanner.mark_pending_notified(state), 1)
        self.assertEqual(state["pending"], {})
        self.assertIn("finding-1", state["notified"])

        third = scanner.update_state_and_filter(
            state,
            [finding()],
            first_run=False,
            notify_on_first_run=False,
        )
        self.assertEqual(third, [])

    def test_can_mark_only_one_pending_chunk_notified(self) -> None:
        state: dict[str, object] = {
            "pending": {
                "finding-1": finding("finding-1"),
                "finding-2": finding("finding-2"),
            },
            "notified": {},
        }

        self.assertEqual(scanner.mark_pending_notified(state, {"finding-1"}), 1)

        self.assertNotIn("finding-1", state["pending"])
        self.assertIn("finding-2", state["pending"])
        self.assertIn("finding-1", state["notified"])
        self.assertNotIn("finding-2", state["notified"])

    def test_first_run_baseline_marks_findings_notified_without_pending(self) -> None:
        state: dict[str, object] = {"seen": {}}

        new_findings = scanner.update_state_and_filter(
            state,
            [finding()],
            first_run=True,
            notify_on_first_run=False,
        )

        self.assertEqual(new_findings, [])
        self.assertEqual(state["pending"], {})
        self.assertIn("finding-1", state["notified"])

    def test_page_terms_finds_monitored_model_near_deprecation_keyword(self) -> None:
        original_fetch = scanner.fetch_url
        body = "<html><body>Upcoming deprecations: gpt-5 will shut down soon.</body></html>"
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 200, {}, body)
        try:
            findings, report = scanner.scan_page_terms(
                {
                    "id": "openai",
                    "name": "OpenAI deprecations",
                    "type": "page_terms",
                    "url": "https://example.com/deprecations",
                    "affected_files": ["agents"],
                    "action": "Review.",
                    "match_terms": ["gpt-5"],
                    "keywords": ["deprecations", "shut down"],
                },
                1,
            )
        finally:
            scanner.fetch_url = original_fetch

        self.assertEqual(len(findings), 1)
        self.assertEqual(report["matched_terms"], ["gpt-5"])

    def test_keyword_matching_uses_boundaries_and_ignores_simple_negation(self) -> None:
        self.assertTrue(scanner.matches_keywords("This release removes a deprecated API.", ["deprecated"]))
        self.assertTrue(scanner.matches_keywords("Breaking changes are planned.", ["breaking change"]))
        self.assertTrue(scanner.matches_keywords("Upcoming deprecations are planned.", ["deprecation"]))
        self.assertFalse(scanner.matches_keywords("This API is no longer deprecated.", ["deprecated"]))
        self.assertFalse(scanner.matches_keywords("This endpoint is not a breaking change.", ["breaking change"]))
        self.assertFalse(scanner.matches_keywords("The flag was undeprecated.", ["deprecated"]))

    def test_kubernetes_api_version_scan_finds_removed_api(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            manifest = root / "manifests" / "pdb.yaml"
            manifest.parent.mkdir()
            manifest.write_text(
                "apiVersion: policy/v1beta1\nkind: PodDisruptionBudget\n",
                encoding="utf-8",
            )

            findings, report = scanner.scan_kubernetes_api_versions(
                {
                    "id": "k8s",
                    "name": "Kubernetes APIs",
                    "type": "kubernetes_api_versions",
                    "paths": ["manifests/*.yaml"],
                    "affected_files": ["manifests/*.yaml"],
                    "action": "Migrate.",
                },
                root=root,
            )

        self.assertEqual(report["checked_files"], 1)
        self.assertEqual(len(findings), 1)
        self.assertIn("policy/v1beta1", findings[0]["title"])

    def test_github_api_versions_finds_missing_pinned_version(self) -> None:
        original_fetch = scanner.fetch_url
        scanner.fetch_url = lambda *args, **kwargs: scanner.FetchResult(
            "https://api.github.com/versions",
            200,
            {},
            '["2026-03-10"]',
        )
        try:
            findings, report = scanner.scan_github_api_versions(
                {
                    "id": "github",
                    "name": "GitHub versions",
                    "type": "github_api_versions",
                    "url": "https://api.github.com/versions",
                    "affected_files": ["workflow.yaml"],
                    "action": "Migrate.",
                    "pinned_version": "2022-11-28",
                },
                1,
            )
        finally:
            scanner.fetch_url = original_fetch

        self.assertEqual(report["latest_supported_version"], "2026-03-10")
        self.assertEqual(len(findings), 1)
        self.assertIn("no longer supported", findings[0]["title"])

    def test_feed_dedup_uses_normalized_link_not_title_or_date(self) -> None:
        original_fetch = scanner.fetch_url
        body = """<?xml version="1.0"?>
        <rss><channel>
          <item>
            <title>Deprecated API notice</title>
            <link>https://example.com/post?utm_source=newsletter#section</link>
            <description>Deprecated API notice.</description>
            <pubDate>Mon, 01 Jun 2026 00:00:00 GMT</pubDate>
          </item>
        </channel></rss>"""
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 200, {}, body)
        try:
            first, _ = scanner.scan_feed(
                {"defaults": {"keywords": ["deprecated"]}},
                {
                    "id": "feed",
                    "name": "Feed",
                    "type": "feed",
                    "url": "https://example.com/feed.xml",
                    "affected_files": ["x"],
                    "action": "Review.",
                },
                1,
            )
        finally:
            scanner.fetch_url = original_fetch

        body = body.replace("Deprecated API notice", "Deprecated API notice updated").replace(
            "Mon, 01 Jun 2026 00:00:00 GMT",
            "Tue, 02 Jun 2026 00:00:00 GMT",
        )
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 200, {}, body)
        try:
            second, _ = scanner.scan_feed(
                {"defaults": {"keywords": ["deprecated"]}},
                {
                    "id": "feed",
                    "name": "Feed",
                    "type": "feed",
                    "url": "https://example.com/feed.xml",
                    "affected_files": ["x"],
                    "action": "Review.",
                },
                1,
            )
        finally:
            scanner.fetch_url = original_fetch

        self.assertEqual(first[0]["id"], second[0]["id"])

    def test_feed_required_terms_reduce_unrelated_matches(self) -> None:
        original_fetch = scanner.fetch_url
        body = """<?xml version="1.0"?>
        <rss><channel>
          <item>
            <title>Deprecated Python runtime</title>
            <link>https://example.com/python</link>
            <description>Python 3.9 is deprecated.</description>
          </item>
          <item>
            <title>Webhook payload deprecated</title>
            <link>https://example.com/webhook</link>
            <description>A webhook payload field is deprecated.</description>
          </item>
        </channel></rss>"""
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 200, {}, body)
        try:
            findings, report = scanner.scan_feed(
                {"defaults": {"keywords": ["deprecated"]}},
                {
                    "id": "feed",
                    "name": "Feed",
                    "type": "feed",
                    "url": "https://example.com/feed.xml",
                    "affected_files": ["x"],
                    "action": "Review.",
                    "required_terms": ["webhook", "payload"],
                },
                1,
            )
        finally:
            scanner.fetch_url = original_fetch

        self.assertEqual(len(findings), 1)
        self.assertEqual(findings[0]["url"], "https://example.com/webhook")
        self.assertEqual(report["required_terms"], ["webhook", "payload"])

    def test_feed_required_terms_must_be_near_keyword(self) -> None:
        original_fetch = scanner.fetch_url
        body = """<?xml version="1.0"?>
        <rss><channel>
          <item>
            <title>Release notes</title>
            <link>https://example.com/release</link>
            <description>Removed an unrelated UI asset. This paragraph is filler text that separates unrelated sections. PromQL query behavior is unchanged.</description>
          </item>
        </channel></rss>"""
        scanner.fetch_url = lambda url, timeout: scanner.FetchResult(url, 200, {}, body)
        try:
            findings, _ = scanner.scan_feed(
                {"defaults": {"keywords": ["removed"]}},
                {
                    "id": "feed",
                    "name": "Feed",
                    "type": "feed",
                    "url": "https://example.com/feed.xml",
                    "affected_files": ["x"],
                    "action": "Review.",
                    "required_terms": ["PromQL"],
                    "required_terms_context_chars": 30,
                },
                1,
            )
        finally:
            scanner.fetch_url = original_fetch

        self.assertEqual(findings, [])

    def test_slack_payloads_are_chunked_without_dropping_findings(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            payload_path = Path(temp_dir) / "slack-payload.json"
            findings = [finding(f"finding-{index}") for index in range(25)]
            scanner.write_slack_payloads(payload_path, findings, False, len(findings))
            payload_files = sorted(scanner.slack_payload_dir(payload_path).glob("*.payload.json"))
            ids_files = sorted(scanner.slack_payload_dir(payload_path).glob("*.ids.json"))

        self.assertEqual(len(payload_files), 3)
        self.assertEqual(len(ids_files), 3)

    def test_validate_config_rejects_dead_affected_file_paths(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            (root / "live.yaml").write_text("kind: ConfigMap\n", encoding="utf-8")
            config = {
                "sources": [
                    {
                        "id": "source",
                        "name": "Source",
                        "type": "reference_page",
                        "url": "https://example.com",
                        "affected_files": ["missing.yaml"],
                        "action": "Review.",
                    }
                ],
                "manifest_scans": [
                    {
                        "id": "scan",
                        "name": "Scan",
                        "type": "kubernetes_api_versions",
                        "paths": ["live.yaml"],
                        "affected_files": ["live.yaml"],
                        "action": "Review.",
                    }
                ],
            }

            errors = scanner.validate_config(config, root=root)

        self.assertIn("source.affected_files: path does not match repository files: missing.yaml", errors)

    def test_source_health_waits_for_repeated_failure(self) -> None:
        state: dict[str, object] = {}
        errors = [{"id": "feed", "url": "https://example.com/feed", "error": "timeout"}]

        first = scanner.source_health_findings(state, errors, {"feed"})
        second = scanner.source_health_findings(state, errors, {"feed"})

        self.assertEqual(first, [])
        self.assertEqual(len(second), 1)
        self.assertIn("consecutive_failures=2", second[0]["summary"])

    def test_source_health_repeats_after_thirty_failures(self) -> None:
        state: dict[str, object] = {
            "source_health": {
                "feed": {
                    "consecutive_failures": 59,
                    "last_error": "timeout",
                    "url": "https://example.com/feed",
                }
            }
        }
        errors = [{"id": "feed", "url": "https://example.com/feed", "error": "timeout"}]

        findings = scanner.source_health_findings(state, errors, {"feed"})

        self.assertEqual(len(findings), 1)
        self.assertIn("consecutive_failures=60", findings[0]["summary"])


if __name__ == "__main__":
    unittest.main()
