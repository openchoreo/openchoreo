#!/usr/bin/env python3
"""Pick the release images for the periodic Trivy scan.

Follows the support policy
(https://openchoreo.dev/docs/releases/release-and-support-process):

- The newest image of each of the latest three GA minors: vX.Y.Z, or a later
  vX.Y.Z-hotfix.N. A hotfix is a pre-release of the next patch, so in semver
  order v1.0.0 < v1.0.1-hotfix.1 < v1.0.1.
- The newest -alpha, -beta, -m (milestone) or -rc of a minor that isn't GA
  yet. Other suffixes are ignored.

Reads `gh release list --json tagName,publishedAt` from stdin and prints the
tags to scan as a JSON list, e.g. ["v1.3.3", "v1.2.8", "v1.1.10", "v1.4.0-m.2"].

Dependency-free so it runs in GitHub Actions with no package install.
"""

from __future__ import annotations

import json
import re
import sys
from dataclasses import dataclass
from typing import Any


SUPPORTED_MINORS = 3
TAG_RE = re.compile(r"^v(\d+)\.(\d+)\.(\d+)(-.+)?$")
HOTFIX_RE = re.compile(r"^-hotfix\.(\d+)$")
PREVIEW_RE = re.compile(r"^-(alpha|beta|m|rc)([.-]\d+)?$")


@dataclass(frozen=True)
class Release:
    tag: str
    major: int
    minor: int
    patch: int
    suffix: str
    published_at: str

    @property
    def minor_line(self) -> tuple[int, int]:
        return (self.major, self.minor)


def parse_releases(releases: list[dict[str, Any]]) -> list[Release]:
    parsed = []
    for release in releases:
        tag = release.get("tagName", "")
        match = TAG_RE.match(tag)
        if not match:
            continue
        major, minor, patch, suffix = match.groups()
        parsed.append(
            Release(tag, int(major), int(minor), int(patch), suffix or "", release.get("publishedAt") or "")
        )
    return parsed


def patch_order(release: Release) -> tuple[int, int, int]:
    """Sort key within a minor: vX.Y.Z-hotfix.N comes before vX.Y.Z."""
    hotfix = HOTFIX_RE.match(release.suffix)
    if hotfix:
        return (release.patch, 0, int(hotfix.group(1)))
    return (release.patch, 1, 0)


def pick_targets(releases: list[dict[str, Any]]) -> list[str]:
    parsed = parse_releases(releases)
    ga_minors = sorted({r.minor_line for r in parsed if not r.suffix}, reverse=True)[:SUPPORTED_MINORS]

    targets = []
    for major, minor in ga_minors:
        candidates = [
            r for r in parsed
            if r.minor_line == (major, minor) and (not r.suffix or HOTFIX_RE.match(r.suffix))
        ]
        newest = max(candidates, key=patch_order)
        targets.append(newest.tag)

    previews = [
        r for r in parsed
        if PREVIEW_RE.match(r.suffix) and (not ga_minors or r.minor_line > ga_minors[0])
    ]
    if previews:
        newest = max(previews, key=lambda r: r.published_at)
        targets.append(newest.tag)

    return targets


def main() -> int:
    json.dump(pick_targets(json.load(sys.stdin)), sys.stdout, separators=(",", ":"))
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
