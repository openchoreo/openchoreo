#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("trivy_scan_targets.py")
SPEC = importlib.util.spec_from_file_location("trivy_scan_targets", SCRIPT_PATH)
assert SPEC is not None
targets = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules[SPEC.name] = targets
SPEC.loader.exec_module(targets)


def release(tag: str, published_at: str) -> dict[str, str]:
    return {"tagName": tag, "publishedAt": published_at}


# A cut of the real release history as of 2026-11-23.
RELEASES = [
    release("v1.4.0-m.2", "2026-11-06T00:00:00Z"),
    release("v1.1.10", "2026-11-20T00:00:00Z"),
    release("v1.4.0-m.1", "2026-10-29T00:00:00Z"),
    release("v1.3.3", "2026-10-29T00:00:00Z"),
    release("v1.2.8", "2026-10-23T00:00:00Z"),
    release("v1.1.9", "2026-10-23T00:00:00Z"),
    release("v1.3.0", "2026-09-19T00:00:00Z"),
    release("v1.3.0-rc.2", "2026-09-18T00:00:00Z"),
    release("v1.2.0", "2026-07-24T00:00:00Z"),
    release("v1.1.0", "2026-05-18T00:00:00Z"),
    release("v1.1.0-alpha-1", "2026-05-12T00:00:00Z"),
    release("v1.0.1", "2026-05-07T00:00:00Z"),
    release("v1.0.1-hotfix.1", "2026-04-16T00:00:00Z"),
    release("v1.0.0", "2026-03-22T00:00:00Z"),
    release("untagged-5f269b0cfe299117dd91", "2025-01-01T00:00:00Z"),
]


class PickTargetsTests(unittest.TestCase):
    def test_latest_three_minors_and_preview(self) -> None:
        self.assertEqual(
            targets.pick_targets(RELEASES),
            [
                "v1.3.3",
                "v1.2.8",
                "v1.1.10",
                "v1.4.0-m.2",
            ],
        )

    def test_hotfix_after_patch_wins(self) -> None:
        picked = targets.pick_targets(RELEASES + [release("v1.3.4-hotfix.1", "2026-11-25T00:00:00Z")])
        self.assertEqual(picked[0], "v1.3.4-hotfix.1")

    def test_patch_wins_over_its_own_hotfix(self) -> None:
        picked = targets.pick_targets(
            [release("v1.0.0", "2026-03-22T00:00:00Z"), release("v1.0.1-hotfix.1", "2026-04-16T00:00:00Z")]
        )
        self.assertEqual(picked, ["v1.0.1-hotfix.1"])
        picked = targets.pick_targets(
            [
                release("v1.0.0", "2026-03-22T00:00:00Z"),
                release("v1.0.1-hotfix.1", "2026-04-16T00:00:00Z"),
                release("v1.0.1", "2026-05-07T00:00:00Z"),
            ]
        )
        self.assertEqual(picked, ["v1.0.1"])

    def test_hotfix_numbers_sort_numerically(self) -> None:
        picked = targets.pick_targets(
            [
                release("v1.3.3", "2026-10-29T00:00:00Z"),
                release("v1.3.4-hotfix.9", "2026-11-25T00:00:00Z"),
                release("v1.3.4-hotfix.10", "2026-11-26T00:00:00Z"),
            ]
        )
        self.assertEqual(picked, ["v1.3.4-hotfix.10"])

    def test_minor_versions_sort_numerically(self) -> None:
        picked = targets.pick_targets(
            [
                release("v1.9.0", "2027-01-01T00:00:00Z"),
                release("v1.10.0", "2027-02-01T00:00:00Z"),
                release("v1.8.0", "2026-12-01T00:00:00Z"),
                release("v1.7.0", "2026-11-01T00:00:00Z"),
            ]
        )
        self.assertEqual(picked, ["v1.10.0", "v1.9.0", "v1.8.0"])

    def test_newest_preview_wins(self) -> None:
        picked = targets.pick_targets(
            RELEASES
            + [
                release("v1.4.0-beta.1", "2026-11-28T00:00:00Z"),
                release("v1.4.0-rc.1", "2026-12-01T00:00:00Z"),
            ]
        )
        self.assertEqual(picked[-1], "v1.4.0-rc.1")

    def test_other_suffixes_are_ignored(self) -> None:
        picked = targets.pick_targets(RELEASES + [release("v1.5.0-dev.1", "2026-12-02T00:00:00Z")])
        self.assertEqual(picked[-1], "v1.4.0-m.2")

    def test_ga_drops_oldest_line_and_preview(self) -> None:
        picked = targets.pick_targets(RELEASES + [release("v1.4.0", "2026-12-10T00:00:00Z")])
        self.assertEqual(
            picked,
            [
                "v1.4.0",
                "v1.3.3",
                "v1.2.8",
            ],
        )

    def test_fewer_than_three_minors(self) -> None:
        picked = targets.pick_targets(
            [release("v1.0.0", "2026-03-22T00:00:00Z"), release("v1.1.0-alpha-1", "2026-05-12T00:00:00Z")]
        )
        self.assertEqual(
            picked,
            [
                "v1.0.0",
                "v1.1.0-alpha-1",
            ],
        )

    def test_no_releases(self) -> None:
        self.assertEqual(targets.pick_targets([]), [])


class CommandLineTests(unittest.TestCase):
    def test_reads_stdin_and_prints_targets(self) -> None:
        result = subprocess.run(
            [sys.executable, str(SCRIPT_PATH)],
            input=json.dumps(RELEASES),
            capture_output=True,
            text=True,
            check=True,
        )
        printed = json.loads(result.stdout)
        self.assertEqual(printed, targets.pick_targets(RELEASES))
        self.assertTrue(all(isinstance(t, str) for t in printed))


if __name__ == "__main__":
    unittest.main()
