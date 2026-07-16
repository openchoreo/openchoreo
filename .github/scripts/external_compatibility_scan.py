#!/usr/bin/env python3
"""Scan curated external compatibility sources and produce a Slack digest.

Dependency-free so it runs in GitHub Actions with no package install. Sources
are declared in the config JSON; each has a `type` handled by a `scan_*`
function (RSS/Atom feeds, provider pages, endoflife.date, GitHub API versions,
response-header probes, and static Kubernetes manifest API-version scans).

State is persisted between runs to dedupe already-seen notices. On the first
run (empty state) findings are baselined rather than sent, so Slack only ever
receives notices that are new since the previous scan.
"""

from __future__ import annotations

import argparse
import hashlib
import html
import json
import os
import re
import sys
import time
import textwrap
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
from dataclasses import dataclass
from datetime import date, datetime, timezone
from pathlib import Path
from typing import Any


USER_AGENT = "openchoreo-external-compatibility-scan/1.0 (+https://github.com/openchoreo/openchoreo)"
MAX_FINDINGS_PER_SLACK_MESSAGE = 10
MAX_SEEN_ITEMS = 2000
RETRYABLE_HTTP_STATUS = {403, 429, 500, 502, 503, 504}
SLACK_SEVERITY_COLORS = {
    "critical": "#d1242f",
    "high": "#fb8500",
    "medium": "#d4a72c",
    "low": "#6e7781",
}
NEGATION_BEFORE_KEYWORD = re.compile(
    r"\b(?:no\s+longer|not(?:\s+an?|\s+any)?|never)\s+(?:\w+\s+){0,3}$",
    re.IGNORECASE,
)
DEFAULT_DEPRECATED_K8S_APIS = {
    "admissionregistration.k8s.io/v1beta1": "1.22",
    "apiextensions.k8s.io/v1beta1": "1.22",
    "apiregistration.k8s.io/v1beta1": "1.22",
    "apps/v1beta1": "1.16",
    "apps/v1beta2": "1.16",
    "autoscaling/v2beta1": "1.25",
    "autoscaling/v2beta2": "1.26",
    "batch/v1beta1": "1.25",
    "certificates.k8s.io/v1beta1": "1.22",
    "coordination.k8s.io/v1beta1": "1.22",
    "discovery.k8s.io/v1beta1": "1.25",
    "events.k8s.io/v1beta1": "1.25",
    "extensions/v1beta1": "1.22",
    "flowcontrol.apiserver.k8s.io/v1beta1": "1.26",
    "flowcontrol.apiserver.k8s.io/v1beta2": "1.29",
    "flowcontrol.apiserver.k8s.io/v1beta3": "1.32",
    "networking.k8s.io/v1beta1": "1.22",
    "node.k8s.io/v1beta1": "1.22",
    "policy/v1beta1": "1.25",
    "rbac.authorization.k8s.io/v1beta1": "1.22",
    "scheduling.k8s.io/v1beta1": "1.22",
    "storage.k8s.io/v1beta1": "1.22",
}


@dataclass
class FetchResult:
    url: str
    status: int
    headers: dict[str, str]
    body: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", required=True, help="Path to external compatibility source JSON")
    parser.add_argument("--state", required=True, help="Path to existing scanner state JSON")
    parser.add_argument("--state-out", required=True, help="Path to write updated state JSON")
    parser.add_argument("--slack-payload", required=True, help="Path to write Slack webhook payload JSON")
    parser.add_argument("--report", required=True, help="Path to write scan report JSON")
    parser.add_argument("--timeout", type=float, default=20.0, help="HTTP timeout in seconds")
    parser.add_argument(
        "--notify-on-first-run",
        action="store_true",
        help="Send matching historical findings when no previous state exists",
    )
    parser.add_argument(
        "--validate-only",
        action="store_true",
        help="Validate config and exit without fetching remote sources",
    )
    parser.add_argument(
        "--mark-notified",
        action="store_true",
        help="Mark the current pending Slack findings as notified and exit",
    )
    parser.add_argument(
        "--mark-notified-ids",
        help="Path to a JSON array of finding IDs to mark as notified",
    )
    return parser.parse_args()


def load_json(path: Path, default: Any) -> Any:
    if not path.exists() or path.stat().st_size == 0:
        return default
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, sort_keys=True)
        handle.write("\n")


def contains_glob(pattern: str) -> bool:
    return any(char in pattern for char in "*?[")


def path_pattern_exists(root: Path, pattern: str) -> bool:
    path = root / pattern
    if contains_glob(pattern):
        return any(root.glob(pattern))
    return path.exists()


def validate_path_patterns(root: Path | None, owner: str, patterns: list[Any], errors: list[str]) -> None:
    if root is None:
        return
    for pattern in patterns:
        pattern_text = str(pattern)
        if not path_pattern_exists(root, pattern_text):
            errors.append(f"{owner}: path does not match repository files: {pattern_text}")


def validate_config(config: dict[str, Any], root: Path | None = None) -> list[str]:
    errors: list[str] = []
    source_ids: set[str] = set()
    for key in ("sources", "header_probes", "manifest_scans"):
        if key in config and not isinstance(config[key], list):
            errors.append(f"{key} must be a list")

    for source in config.get("sources", []):
        source_id = source.get("id")
        if not source_id:
            errors.append("source is missing id")
        elif source_id in source_ids:
            errors.append(f"duplicate source id: {source_id}")
        else:
            source_ids.add(source_id)
        if source.get("type") not in {
            "feed",
            "github_api_versions",
            "page_contains",
            "page_terms",
            "reference_page",
            "eol_api",
        }:
            errors.append(
                f"{source_id}: type must be feed, github_api_versions, "
                "page_contains, page_terms, reference_page, or eol_api"
            )
        if not source.get("url"):
            errors.append(f"{source_id}: missing url")
        if not source.get("affected_files"):
            errors.append(f"{source_id}: missing affected_files")
        else:
            validate_path_patterns(root, f"{source_id}.affected_files", source.get("affected_files", []), errors)
        if not source.get("action"):
            errors.append(f"{source_id}: missing action")
        if source.get("type") == "page_contains" and not source.get("required_text"):
            errors.append(f"{source_id}: page_contains sources require required_text")
        if source.get("type") == "page_terms" and not source.get("match_terms"):
            errors.append(f"{source_id}: page_terms sources require match_terms")
        if source.get("required_terms") is not None and not isinstance(source.get("required_terms"), list):
            errors.append(f"{source_id}: required_terms must be a list")
        if not isinstance(source.get("required_terms_context_chars", 350), int):
            errors.append(f"{source_id}: required_terms_context_chars must be an integer")
        if source.get("type") == "github_api_versions" and not source.get("pinned_version"):
            errors.append(f"{source_id}: github_api_versions sources require pinned_version")
        if source.get("type") == "eol_api":
            if not source.get("cycle"):
                errors.append(f"{source_id}: eol_api sources require cycle")
            if not isinstance(source.get("warn_within_days", 180), int):
                errors.append(f"{source_id}: warn_within_days must be an integer")

    probe_ids: set[str] = set()
    for probe in config.get("header_probes", []):
        probe_id = probe.get("id")
        if not probe_id:
            errors.append("header probe is missing id")
        elif probe_id in probe_ids:
            errors.append(f"duplicate header probe id: {probe_id}")
        else:
            probe_ids.add(probe_id)
        if not probe.get("url"):
            errors.append(f"{probe_id}: missing url")
        if not probe.get("watched_headers"):
            errors.append(f"{probe_id}: missing watched_headers")
        if not probe.get("affected_files"):
            errors.append(f"{probe_id}: missing affected_files")
        else:
            validate_path_patterns(root, f"{probe_id}.affected_files", probe.get("affected_files", []), errors)
        if not probe.get("action"):
            errors.append(f"{probe_id}: missing action")

    manifest_ids: set[str] = set()
    for scan in config.get("manifest_scans", []):
        scan_id = scan.get("id")
        if not scan_id:
            errors.append("manifest scan is missing id")
        elif scan_id in manifest_ids:
            errors.append(f"duplicate manifest scan id: {scan_id}")
        else:
            manifest_ids.add(scan_id)
        if scan.get("type") != "kubernetes_api_versions":
            errors.append(f"{scan_id}: type must be kubernetes_api_versions")
        if not scan.get("paths"):
            errors.append(f"{scan_id}: missing paths")
        else:
            validate_path_patterns(root, f"{scan_id}.paths", scan.get("paths", []), errors)
        if not scan.get("affected_files"):
            errors.append(f"{scan_id}: missing affected_files")
        else:
            validate_path_patterns(root, f"{scan_id}.affected_files", scan.get("affected_files", []), errors)
        if not scan.get("action"):
            errors.append(f"{scan_id}: missing action")

    return errors


def fetch_url(
    url: str,
    timeout: float,
    method: str = "GET",
    request_headers: dict[str, str] | None = None,
    auth_env: str | None = None,
) -> FetchResult:
    headers = {"User-Agent": USER_AGENT}
    headers.update(request_headers or {})

    if auth_env:
        token = os.getenv(auth_env)
        if token and "Authorization" not in headers:
            headers["Authorization"] = f"Bearer {token}"

    last_error: Exception | None = None
    attempts = 3
    for attempt in range(attempts):
        request = urllib.request.Request(url, method=method, headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=timeout) as response:
                raw = response.read()
                charset = response.headers.get_content_charset() or "utf-8"
                body = raw.decode(charset, errors="replace")
                return FetchResult(
                    url=url,
                    status=response.status,
                    headers={key.lower(): value for key, value in response.headers.items()},
                    body=body,
                )
        except urllib.error.HTTPError as exc:
            if exc.code not in RETRYABLE_HTTP_STATUS or attempt == attempts - 1:
                raw = exc.read()
                charset = exc.headers.get_content_charset() or "utf-8"
                return FetchResult(
                    url=url,
                    status=exc.code,
                    headers={key.lower(): value for key, value in exc.headers.items()},
                    body=raw.decode(charset, errors="replace"),
                )
            last_error = exc
            time.sleep(retry_delay_seconds(exc.headers.get("Retry-After"), attempt))
        except urllib.error.URLError as exc:
            if attempt == attempts - 1:
                raise
            last_error = exc
            time.sleep(retry_delay_seconds(None, attempt))

    raise last_error or RuntimeError(f"failed to fetch {url}")


def retry_delay_seconds(retry_after: str | None, attempt: int) -> float:
    if retry_after:
        try:
            return min(float(retry_after), 10.0)
        except ValueError:
            pass
    return min(0.5 * (2 ** attempt), 4.0)


def require_success(result: FetchResult) -> None:
    if result.status < 200 or result.status >= 300:
        raise ValueError(f"{result.url} returned HTTP {result.status}")


def normalized_text_from_html(body: str) -> str:
    body = re.sub(r"(?is)<(script|style).*?>.*?</\1>", " ", body)
    body = re.sub(r"(?is)<[^>]+>", " ", body)
    body = html.unescape(body)
    return re.sub(r"\s+", " ", body).strip()


def keyword_patterns(keywords: list[str]) -> list[re.Pattern[str]]:
    patterns: list[re.Pattern[str]] = []
    for keyword in keywords:
        variants = [keyword]
        if keyword.lower() in {"breaking change", "deprecation", "removal", "retirement"}:
            variants.append(f"{keyword}s")
        alternatives = "|".join(re.escape(variant) for variant in variants)
        patterns.append(re.compile(rf"(?<![A-Za-z0-9])(?:{alternatives})(?![A-Za-z0-9])", re.IGNORECASE))
    return patterns


def matches_keywords(text: str, keywords: list[str]) -> bool:
    for pattern in keyword_patterns(keywords):
        for match in pattern.finditer(text):
            if not is_negated_keyword_match(text, match):
                return True
    return False


def matches_keywords_near_required_terms(
    text: str,
    keywords: list[str],
    required_terms: list[str],
    context_chars: int = 350,
) -> bool:
    for pattern in keyword_patterns(keywords):
        for match in pattern.finditer(text):
            if is_negated_keyword_match(text, match):
                continue
            start = max(0, match.start() - context_chars)
            end = min(len(text), match.end() + context_chars)
            if matches_keywords(text[start:end], required_terms):
                return True
    return False


def is_negated_keyword_match(text: str, match: re.Match[str]) -> bool:
    before = text[max(0, match.start() - 80):match.start()]
    if before.lower().endswith("un-"):
        return True
    return bool(NEGATION_BEFORE_KEYWORD.search(before))


def stable_hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()[:24]


def finding_id(*parts: str) -> str:
    return stable_hash("\0".join(parts))


def normalize_feed_link(link: str) -> str:
    parsed = urllib.parse.urlsplit(link.strip())
    if not parsed.scheme or not parsed.netloc:
        return link.strip()
    query = urllib.parse.parse_qsl(parsed.query, keep_blank_values=True)
    filtered_query = [
        (key, value)
        for key, value in query
        if not key.lower().startswith("utm_") and key.lower() not in {"fbclid", "gclid"}
    ]
    return urllib.parse.urlunsplit(
        (
            parsed.scheme.lower(),
            parsed.netloc.lower(),
            parsed.path.rstrip("/") or "/",
            urllib.parse.urlencode(filtered_query),
            "",
        )
    )


def parse_feed_items(body: str) -> list[dict[str, str]]:
    try:
        root = ET.fromstring(body)
    except ET.ParseError as exc:
        raise ValueError(f"feed XML parse failed: {exc}") from exc

    items: list[dict[str, str]] = []
    for node in root.findall(".//item"):
        items.append(
            {
                "title": text_of(node, "title"),
                "link": text_of(node, "link"),
                "summary": text_of(node, "description"),
                "published": text_of(node, "pubDate"),
            }
        )

    ns = {"atom": "http://www.w3.org/2005/Atom"}
    for node in root.findall(".//atom:entry", ns):
        link = ""
        for link_node in node.findall("atom:link", ns):
            if link_node.attrib.get("href"):
                link = link_node.attrib["href"]
                break
        items.append(
            {
                "title": text_of(node, "atom:title", ns),
                "link": link,
                "summary": text_of(node, "atom:summary", ns) or text_of(node, "atom:content", ns),
                "published": text_of(node, "atom:updated", ns) or text_of(node, "atom:published", ns),
            }
        )
    return items


def text_of(node: ET.Element, path: str, namespaces: dict[str, str] | None = None) -> str:
    child = node.find(path, namespaces or {})
    if child is None or child.text is None:
        return ""
    return re.sub(r"\s+", " ", html.unescape(child.text)).strip()


def source_keywords(config: dict[str, Any], source: dict[str, Any]) -> list[str]:
    defaults = config.get("defaults", {}).get("keywords", [])
    keywords = source.get("keywords") or defaults
    return [str(keyword).lower() for keyword in keywords]


def common_finding_fields(source: dict[str, Any]) -> dict[str, Any]:
    return {
        "source_id": source["id"],
        "source_name": source["name"],
        "severity": source.get("severity", "medium"),
        "owner": source.get("owner", "unassigned"),
        "affected_files": source.get("affected_files", []),
        "action": source.get("action", "Review the linked source and assess impact."),
    }


def scan_reference_page(source: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Fetch a page for source-health only; it never produces findings.

    Used for reference docs where keyword matching would be too noisy. The
    returned report records a content hash so drift is visible in report.json.
    """
    result = fetch_url(source["url"], timeout)
    require_success(result)
    text = normalized_text_from_html(result.body)
    return [], {
        "status": result.status,
        "content_hash": stable_hash(text),
        "matches": 0,
        "url": source["url"],
    }


def scan_page_contains(source: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Alert when expected text disappears from a provider page.

    Inverts the usual keyword scan: a finding is raised when a required_text
    value (e.g. a currently-supported version) is no longer present.
    """
    result = fetch_url(source["url"], timeout)
    require_success(result)
    text = normalized_text_from_html(result.body).lower()
    missing = [needle for needle in source.get("required_text", []) if str(needle).lower() not in text]
    findings: list[dict[str, Any]] = []
    if missing:
        title = f"{source['name']} no longer contains expected supported value(s)"
        summary = "Missing expected text: " + ", ".join(missing)
        findings.append(
            {
                **common_finding_fields(source),
                "id": finding_id(source["id"], source["url"], json.dumps(sorted(missing))),
                "title": title,
                "url": source["url"],
                "summary": summary,
                "observed_at": now_iso(),
                "kind": "page_contains",
            }
        )
    return findings, {
        "status": result.status,
        "required_text": source.get("required_text", []),
        "missing": missing,
        "matches": len(findings),
        "url": source["url"],
    }


def source_request_headers(source: dict[str, Any]) -> dict[str, str]:
    return {str(key): str(value) for key, value in source.get("request_headers", {}).items()}


def scan_github_api_versions(source: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Check the pinned GitHub REST API version against the supported list.

    Raises findings when the pinned version has dropped off the supported list,
    when its `supported_until` window is closing, or (optionally) when it is no
    longer the latest supported version.
    """
    result = fetch_url(
        source["url"],
        timeout,
        request_headers=source_request_headers(source),
        auth_env=source.get("auth_env"),
    )
    require_success(result)
    versions = json.loads(result.body)
    if not isinstance(versions, list):
        raise ValueError("GitHub versions response must be a list")
    supported_versions = [str(version) for version in versions]
    pinned = str(source["pinned_version"])
    latest = max(supported_versions) if supported_versions else ""
    findings: list[dict[str, Any]] = []

    if pinned not in supported_versions:
        findings.append(
            {
                **common_finding_fields(source),
                "id": finding_id(source["id"], pinned, "missing"),
                "title": f"GitHub REST API version {pinned} is no longer supported",
                "url": source["url"],
                "summary": f"Supported versions from GitHub: {', '.join(supported_versions)}",
                "observed_at": now_iso(),
                "kind": "github_api_versions",
            }
        )

    supported_until = source.get("supported_until")
    days_until_eol: int | None = None
    if supported_until:
        eol_date = date.fromisoformat(str(supported_until))
        days_until_eol = (eol_date - datetime.now(timezone.utc).date()).days
        if days_until_eol <= int(source.get("warn_within_days", 180)):
            findings.append(
                {
                    **common_finding_fields(source),
                    "id": finding_id(source["id"], pinned, str(supported_until)),
                    "title": f"GitHub REST API version {pinned} support window is closing",
                    "url": source["url"],
                    "summary": (
                        f"pinned_version={pinned}, supported_until={supported_until}, "
                        f"days_until_eol={days_until_eol}"
                    ),
                    "observed_at": now_iso(),
                    "kind": "github_api_versions",
                }
            )

    if source.get("warn_if_not_latest") and latest and pinned != latest:
        findings.append(
            {
                **common_finding_fields(source),
                "id": finding_id(source["id"], pinned, latest, "not-latest"),
                "title": f"GitHub REST API version {pinned} is not the latest supported version",
                "url": source["url"],
                "summary": f"latest_supported_version={latest}, supported_versions={', '.join(supported_versions)}",
                "observed_at": now_iso(),
                "kind": "github_api_versions",
            }
        )

    return findings, {
        "status": result.status,
        "pinned_version": pinned,
        "latest_supported_version": latest,
        "supported_versions": supported_versions,
        "supported_until": supported_until,
        "days_until_eol": days_until_eol,
        "matches": len(findings),
        "url": source["url"],
    }


def scan_page_terms(source: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Alert when a monitored term appears on a page near a change keyword.

    A term only produces a finding if a deprecation/removal keyword also occurs
    within `context_chars` of it, keeping incidental mentions out of Slack.
    """
    result = fetch_url(source["url"], timeout)
    require_success(result)
    text = normalized_text_from_html(result.body)
    lowered = text.lower()
    keywords = source_keywords({"defaults": {"keywords": source.get("keywords", [])}}, source)
    context_chars = int(source.get("context_chars", 500))
    findings: list[dict[str, Any]] = []
    matched_terms: list[str] = []
    for term in source.get("match_terms", []):
        term_text = str(term)
        index = lowered.find(term_text.lower())
        if index < 0:
            continue
        start = max(0, index - context_chars)
        end = min(len(text), index + len(term_text) + context_chars)
        context = text[start:end].strip()
        if keywords and not matches_keywords(context, keywords):
            continue
        matched_terms.append(term_text)
        findings.append(
            {
                **common_finding_fields(source),
                "id": finding_id(source["id"], source["url"], term_text),
                "title": f"{source['name']} mentions monitored term: {term_text}",
                "url": source["url"],
                "summary": context[:700],
                "observed_at": now_iso(),
                "kind": "page_terms",
            }
        )
    return findings, {
        "status": result.status,
        "match_terms": source.get("match_terms", []),
        "matched_terms": matched_terms,
        "matches": len(findings),
        "url": source["url"],
    }


def scan_eol_api(source: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Check a cycle's end-of-life date from an endoflife.date response.

    Alerts when the cycle is already EOL, or when its EOL date is within
    `warn_within_days`.
    """
    result = fetch_url(source["url"], timeout)
    require_success(result)
    payload = json.loads(result.body)
    if not isinstance(payload, list):
        raise ValueError("endoflife.date response must be a list")

    cycle = str(source["cycle"])
    warn_within_days = int(source.get("warn_within_days", 180))
    matching = next((item for item in payload if str(item.get("cycle")) == cycle), None)
    if not matching:
        raise ValueError(f"cycle {cycle} not found in {source['url']}")

    eol_value = matching.get("eol")
    findings: list[dict[str, Any]] = []
    days_until_eol: int | None = None
    should_alert = False
    title = f"{source['name']} cycle {cycle} end-of-life status"
    summary = f"cycle={cycle}, eol={eol_value}"

    if eol_value is True:
        should_alert = True
        summary += ", status=already EOL"
    elif isinstance(eol_value, str):
        eol_date = date.fromisoformat(eol_value)
        days_until_eol = (eol_date - datetime.now(timezone.utc).date()).days
        should_alert = days_until_eol <= warn_within_days
        summary += f", days_until_eol={days_until_eol}, warn_within_days={warn_within_days}"

    if should_alert:
        findings.append(
            {
                **common_finding_fields(source),
                "id": finding_id(source["id"], cycle, str(eol_value)),
                "title": title,
                "url": source["url"],
                "summary": summary,
                "observed_at": now_iso(),
                "kind": "eol_api",
            }
        )

    return findings, {
        "status": result.status,
        "cycle": cycle,
        "eol": eol_value,
        "days_until_eol": days_until_eol,
        "matches": len(findings),
        "url": source["url"],
    }


def scan_feed(config: dict[str, Any], source: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Scan the newest RSS/Atom items for change keywords.

    Each item's title and summary are matched against the source's keywords. If
    `required_terms` is set, a keyword only counts when one of those terms also
    appears nearby, narrowing a broad feed to items relevant to OpenChoreo.
    """
    result = fetch_url(source["url"], timeout)
    require_success(result)
    keywords = source_keywords(config, source)
    required_terms = [str(term).lower() for term in source.get("required_terms", [])]
    required_terms_context_chars = int(source.get("required_terms_context_chars", 350))
    findings: list[dict[str, Any]] = []
    items = parse_feed_items(result.body)
    if not items:
        raise ValueError(f"{source['url']} contained no RSS/Atom items")
    for item in items[:50]:
        haystack = " ".join([item.get("title", ""), item.get("summary", "")])
        if required_terms:
            if not matches_keywords_near_required_terms(
                haystack,
                keywords,
                required_terms,
                context_chars=required_terms_context_chars,
            ):
                continue
        elif not matches_keywords(haystack, keywords):
            continue
        link = item.get("link") or source["url"]
        title = item.get("title") or source["name"]
        summary = normalized_text_from_html(item.get("summary", ""))[:700]
        dedupe_key = normalize_feed_link(link) if item.get("link") else f"{source['url']}#{title}"
        findings.append(
            {
                **common_finding_fields(source),
                "id": finding_id(source["id"], dedupe_key),
                "title": title,
                "url": link,
                "summary": summary,
                "published": item.get("published", ""),
                "observed_at": now_iso(),
                "kind": "feed",
            }
        )
    return findings, {
        "status": result.status,
        "items": len(items),
        "matches": len(findings),
        "required_terms": source.get("required_terms", []),
        "required_terms_context_chars": required_terms_context_chars if required_terms else None,
        "url": source["url"],
    }


def scan_header_probe(probe: dict[str, Any], timeout: float) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Probe an endpoint for RFC 8594 deprecation signals.

    Raises a finding when any watched response header (Deprecation, Sunset,
    Warning, ...) is present, or when the endpoint returns HTTP 410 Gone.
    """
    result = fetch_url(
        probe["url"],
        timeout,
        method=probe.get("method", "GET"),
        request_headers=probe.get("request_headers", {}),
        auth_env=probe.get("auth_env"),
    )
    if result.status != 410:
        require_success(result)
    headers_found: dict[str, str] = {}
    for header in probe.get("watched_headers", []):
        value = result.headers.get(header.lower())
        if value:
            headers_found[header] = value

    findings: list[dict[str, Any]] = []
    if headers_found or result.status == 410:
        title = f"{probe['name']} returned deprecation-related response signal"
        summary_parts = [f"HTTP status: {result.status}"]
        for header, value in sorted(headers_found.items()):
            summary_parts.append(f"{header}: {value}")
        findings.append(
            {
                "source_id": probe["id"],
                "source_name": probe["name"],
                "severity": "critical" if result.status == 410 else probe.get("severity", "high"),
                "owner": probe.get("owner", "unassigned"),
                "affected_files": probe.get("affected_files", []),
                "action": probe.get("action", "Review the response headers and migrate before the sunset date."),
                "id": finding_id(probe["id"], probe["url"], str(result.status), json.dumps(headers_found, sort_keys=True)),
                "title": title,
                "url": probe["url"],
                "summary": "; ".join(summary_parts),
                "observed_at": now_iso(),
                "kind": "header_probe",
            }
        )

    return findings, {"status": result.status, "headers_found": headers_found, "url": probe["url"]}


def scan_kubernetes_api_versions(scan: dict[str, Any], root: Path | None = None) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Statically scan local manifests for removed Kubernetes API versions.

    Reads each matched file's `apiVersion:` lines and flags any that appear in
    the removed-API table. Templated values (containing `{{`) are skipped.
    """
    root = root or Path.cwd()
    deprecated = dict(DEFAULT_DEPRECATED_K8S_APIS)
    deprecated.update(scan.get("deprecated_api_versions", {}))
    api_pattern = re.compile(r"^\s*apiVersion:\s*[\"']?([^\"'\s#]+)", re.MULTILINE)
    findings: list[dict[str, Any]] = []
    checked_files = 0
    for pattern in scan.get("paths", []):
        for path in sorted(root.glob(str(pattern))):
            if not path.is_file():
                continue
            checked_files += 1
            text = path.read_text(encoding="utf-8", errors="replace")
            for match in api_pattern.finditer(text):
                api_version = match.group(1)
                if "{{" in api_version or api_version not in deprecated:
                    continue
                line = text.count("\n", 0, match.start()) + 1
                removed_in = deprecated[api_version]
                findings.append(
                    {
                        "source_id": scan["id"],
                        "source_name": scan["name"],
                        "severity": scan.get("severity", "high"),
                        "owner": scan.get("owner", "platform"),
                        "affected_files": scan.get("affected_files", []),
                        "action": scan.get("action", "Migrate the manifest to a supported Kubernetes API version."),
                        "id": finding_id(scan["id"], str(path.relative_to(root)), str(line), api_version, str(removed_in)),
                        "title": f"{api_version} is removed in Kubernetes {removed_in}",
                        "url": str(path.relative_to(root)),
                        "summary": f"{path.relative_to(root)}:{line} uses apiVersion {api_version}, removed in Kubernetes {removed_in}.",
                        "observed_at": now_iso(),
                        "kind": "kubernetes_api_version",
                    }
                )
    return findings, {
        "checked_files": checked_files,
        "matches": len(findings),
        "paths": scan.get("paths", []),
    }


def source_health_findings(state: dict[str, Any], errors: list[dict[str, str]], checked_ids: set[str]) -> list[dict[str, Any]]:
    """Track per-source failures and alert once they persist.

    A source that succeeds has its failure counter reset. A failing source only
    raises a finding at escalating thresholds (2, 3, 7, 14, 30 consecutive
    failures, then every 30) so a single transient outage stays quiet while a
    genuinely dark source is surfaced.
    """
    health = state.setdefault("source_health", {})
    by_id = {str(error.get("id") or "unknown-source"): error for error in errors}
    findings: list[dict[str, Any]] = []
    for source_id in checked_ids:
        if source_id not in by_id:
            if source_id in health:
                health[source_id]["consecutive_failures"] = 0
                health[source_id]["last_success_at"] = now_iso()
            continue

        error = by_id[source_id]
        url = str(error.get("url") or "")
        message = str(error.get("error") or "unknown error")
        previous = health.get(source_id, {})
        previous_message = previous.get("last_error")
        consecutive = int(previous.get("consecutive_failures", 0)) + 1
        health[source_id] = {
            "consecutive_failures": consecutive,
            "last_error": message,
            "last_failure_at": now_iso(),
            "url": url,
        }

        alert_counts = {2, 3, 7, 14, 30}
        if consecutive not in alert_counts and consecutive % 30 != 0:
            continue

        findings.append(
            {
                "source_id": f"{source_id}-source-health",
                "source_name": f"Source health: {source_id}",
                "severity": "high",
                "owner": "platform",
                "affected_files": [
                    ".github/external-compatibility-sources.json",
                    ".github/workflows/external-compatibility-scan.yaml",
                ],
                "action": "Fix or remove the monitored source so external compatibility coverage does not silently go dark.",
                "id": finding_id("source_error", source_id, url, message, str(consecutive)),
                "title": "External compatibility scan source failed",
                "url": url,
                "summary": f"{message}; consecutive_failures={consecutive}",
                "observed_at": now_iso(),
                "kind": "source_error",
            }
        )
    return findings


def now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def trim_state(state: dict[str, Any]) -> dict[str, Any]:
    seen = state.get("seen", {})
    if len(seen) <= MAX_SEEN_ITEMS:
        return state
    ordered = sorted(seen.items(), key=lambda item: item[1].get("last_seen_at", ""))
    state["seen"] = dict(ordered[-MAX_SEEN_ITEMS:])
    return state


def update_state_and_filter(
    state: dict[str, Any],
    findings: list[dict[str, Any]],
    first_run: bool,
    notify_on_first_run: bool,
) -> list[dict[str, Any]]:
    """Record findings in state and return the ones that should be notified.

    A finding is notified unless it was already notified before, or this is the
    first run and first-run notification is disabled — in which case it is
    baselined (marked notified without sending). `seen` retains every finding's
    first/last observation for auditing.
    """
    seen = state.setdefault("seen", {})
    notified = state.setdefault("notified", {})
    pending = state.setdefault("pending", {})
    new_findings: list[dict[str, Any]] = []
    for finding in findings:
        existing = seen.get(finding["id"])
        should_notify = finding["id"] not in notified and (notify_on_first_run or not first_run)
        if should_notify:
            new_findings.append(finding)
            pending[finding["id"]] = finding
        elif first_run and not notify_on_first_run:
            notified[finding["id"]] = {
                "source_id": finding["source_id"],
                "title": finding["title"],
                "url": finding["url"],
                "baselined_at": finding["observed_at"],
            }
        seen[finding["id"]] = {
            "source_id": finding["source_id"],
            "title": finding["title"],
            "url": finding["url"],
            "first_seen_at": existing.get("first_seen_at") if existing else finding["observed_at"],
            "last_seen_at": finding["observed_at"],
        }
    state["last_scan_at"] = now_iso()
    state["initialized"] = True
    return new_findings


def pending_findings(state: dict[str, Any]) -> list[dict[str, Any]]:
    pending = state.get("pending", {})
    if not isinstance(pending, dict):
        return []
    return [finding for _, finding in sorted(pending.items(), key=lambda item: item[1].get("observed_at", ""))]


def mark_pending_notified(state: dict[str, Any], finding_ids: set[str] | None = None) -> int:
    """Move pending findings to notified after Slack delivery succeeds.

    Called by --mark-notified once the workflow has posted a chunk, so a failed
    post leaves findings pending and they are retried on the next run. With
    finding_ids given, only those are marked; otherwise all pending are marked.
    """
    pending = state.setdefault("pending", {})
    notified = state.setdefault("notified", {})
    marked = 0
    for finding_id_value, finding in list(pending.items()):
        if finding_ids is not None and finding_id_value not in finding_ids:
            continue
        notified[finding_id_value] = {
            "source_id": finding.get("source_id"),
            "title": finding.get("title"),
            "url": finding.get("url"),
            "notified_at": now_iso(),
        }
        del pending[finding_id_value]
        marked += 1
    return marked


def workflow_run_url() -> str:
    server = os.getenv("GITHUB_SERVER_URL")
    repository = os.getenv("GITHUB_REPOSITORY")
    run_id = os.getenv("GITHUB_RUN_ID")
    if not server or not repository or not run_id:
        return ""
    return f"{server.rstrip('/')}/{repository}/actions/runs/{run_id}"


def slack_mrkdwn_escape(value: Any) -> str:
    return str(value).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def slack_link(url: str, label: str = "Open source") -> str:
    if not url.startswith(("http://", "https://")):
        return slack_mrkdwn_escape(url)
    return f"<{url}|{slack_mrkdwn_escape(label)}>"


def is_http_url(url: str) -> bool:
    return url.startswith(("http://", "https://"))


def slack_severity_color(severity: str) -> str:
    return SLACK_SEVERITY_COLORS.get(severity.lower(), "#6e7781")


def slack_finding_attachment(finding: dict[str, Any]) -> dict[str, Any]:
    affected_files = finding.get("affected_files", [])
    affected = ", ".join(affected_files[:3]) if affected_files else "Not specified"
    if len(affected_files) > 3:
        affected += ", ..."

    severity = str(finding.get("severity", "medium")).upper()
    source_name = slack_mrkdwn_escape(finding.get("source_name", "Unknown source"))
    title = slack_mrkdwn_escape(finding.get("title", "Untitled finding"))
    action = slack_mrkdwn_escape(finding.get("action", "Review the source and assess impact."))
    owner = slack_mrkdwn_escape(finding.get("owner", "unassigned"))
    url = str(finding.get("url", ""))
    first_block: dict[str, Any] = {
        "type": "section",
        "text": {
            "type": "mrkdwn",
            "text": f"*{title}*\n{source_name}",
        },
    }
    if is_http_url(url):
        first_block["accessory"] = {
            "type": "button",
            "text": {
                "type": "plain_text",
                "text": "View notice",
            },
            "url": url,
        }

    return {
        "color": slack_severity_color(str(finding.get("severity", "medium"))),
        "fallback": f"{severity} - {finding.get('source_name', 'Unknown source')}: {finding.get('title', 'Untitled finding')}",
        "blocks": [
            first_block,
            {
                "type": "section",
                "fields": [
                    {
                        "type": "mrkdwn",
                        "text": f"*Severity*\n{severity}",
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Owner*\n{owner}",
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Affected*\n{slack_mrkdwn_escape(affected)}",
                    },
                    {
                        "type": "mrkdwn",
                        "text": f"*Source*\n{slack_link(url, 'Notice link')}",
                    },
                ],
            },
            {
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": f"*Action*\n{action}",
                },
            },
        ],
    }


def slack_payload(
    findings: list[dict[str, Any]],
    first_run: bool,
    total_matches: int,
    part: int = 1,
    total_parts: int = 1,
) -> dict[str, Any]:
    first_run_note = " First scan state for this branch." if first_run else ""
    source_errors = sum(1 for finding in findings if finding.get("kind") == "source_error")
    compatibility_findings = len(findings) - source_errors
    part_note = f" Part {part}/{total_parts}." if total_parts > 1 else ""
    title = "OpenChoreo External Compatibility Scan"
    summary = "External API, webhook, SaaS, lifecycle, and Kubernetes compatibility notices."
    fallback_lines = [
        f"{title}: {compatibility_findings} compatibility notice(s), {source_errors} source-health issue(s).{first_run_note}{part_note}",
        f"Total matching notices observed this run: {total_matches}.",
        "See the external-compatibility-scan-state artifact for report.json and full details.",
    ]
    blocks: list[dict[str, Any]] = [
        {
            "type": "header",
            "text": {
                "type": "plain_text",
                "text": title,
            },
        },
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": summary,
            },
        },
        {
            "type": "section",
            "fields": [
                {
                    "type": "mrkdwn",
                    "text": f"*Compatibility notices*\n{compatibility_findings}",
                },
                {
                    "type": "mrkdwn",
                    "text": f"*Source-health issues*\n{source_errors}",
                },
                {
                    "type": "mrkdwn",
                    "text": f"*Total matches this run*\n{total_matches}",
                },
                {
                    "type": "mrkdwn",
                    "text": f"*Run context*\n{('First scan state' if first_run else 'Existing scan state')}{part_note}",
                },
            ],
        },
        {
            "type": "context",
            "elements": [
                {
                    "type": "mrkdwn",
                    "text": "Full details are in the `external-compatibility-scan-state` artifact.",
                }
            ],
        },
    ]
    run_url = workflow_run_url()
    if run_url:
        fallback_lines.append(f"Workflow run: {run_url}")
        blocks.append(
            {
                "type": "context",
                "elements": [
                    {
                        "type": "mrkdwn",
                        "text": f"Workflow run: {slack_link(run_url, 'open in GitHub Actions')}",
                    }
                ],
            }
        )
    blocks.append({"type": "divider"})
    for finding in findings:
        affected = ", ".join(finding.get("affected_files", [])[:3])
        if len(finding.get("affected_files", [])) > 3:
            affected += ", ..."
        fallback_lines.append(
            "\n".join(
                [
                    "",
                    f"*{finding['severity'].upper()}* - {finding['source_name']}",
                    finding["title"],
                    f"Source: {finding['url']}",
                    f"Affected: {affected}",
                    f"Owner: {finding.get('owner', 'unassigned')}",
                    f"Action: {finding['action']}",
                ]
            )
        )
    return {
        "text": "\n".join(fallback_lines),
        "blocks": blocks,
        "attachments": [slack_finding_attachment(finding) for finding in findings],
    }


def slack_payload_dir(slack_payload_path: Path) -> Path:
    return slack_payload_path.parent / f"{slack_payload_path.stem}s"


def write_slack_payloads(
    slack_payload_path: Path,
    findings: list[dict[str, Any]],
    first_run: bool,
    total_matches: int,
) -> None:
    """Write Slack payloads, chunked to stay under Slack's per-message limits.

    Findings are split into groups of MAX_FINDINGS_PER_SLACK_MESSAGE; each chunk
    gets a numbered payload plus a matching `.ids.json` the workflow feeds back
    to --mark-notified-ids after posting. The stale payload dir is cleared first,
    and the first chunk is also written to the primary payload path.
    """
    payload_dir = slack_payload_dir(slack_payload_path)
    payload_dir.mkdir(parents=True, exist_ok=True)
    for existing in payload_dir.glob("*.json"):
        existing.unlink()

    if not findings:
        if slack_payload_path.exists():
            slack_payload_path.unlink()
        return

    chunks = [
        findings[index:index + MAX_FINDINGS_PER_SLACK_MESSAGE]
        for index in range(0, len(findings), MAX_FINDINGS_PER_SLACK_MESSAGE)
    ]
    total_parts = len(chunks)
    for index, chunk in enumerate(chunks, start=1):
        payload = slack_payload(chunk, first_run, total_matches, part=index, total_parts=total_parts)
        payload_path = payload_dir / f"{index:03d}.payload.json"
        ids_path = payload_dir / f"{index:03d}.ids.json"
        write_json(payload_path, payload)
        write_json(ids_path, [finding["id"] for finding in chunk])
        if index == 1:
            write_json(slack_payload_path, payload)


def main() -> int:
    args = parse_args()
    config_path = Path(args.config)
    state_path = Path(args.state)
    state_out_path = Path(args.state_out)
    slack_payload_path = Path(args.slack_payload)
    report_path = Path(args.report)

    config = load_json(config_path, {})
    errors = validate_config(config, root=Path.cwd())
    if errors:
        for error in errors:
            print(f"config error: {error}", file=sys.stderr)
        return 2
    if args.validate_only:
        print(
            f"validated {len(config.get('sources', []))} sources, "
            f"{len(config.get('manifest_scans', []))} manifest scans, "
            f"and {len(config.get('header_probes', []))} header probes"
        )
        return 0

    state = load_json(state_path, {"seen": {}})
    if args.mark_notified or args.mark_notified_ids:
        finding_ids = None
        if args.mark_notified_ids:
            finding_ids = {str(finding_id_value) for finding_id_value in load_json(Path(args.mark_notified_ids), [])}
        marked = mark_pending_notified(state, finding_ids=finding_ids)
        state["last_notification_at"] = now_iso()
        write_json(state_out_path, state)
        print(f"marked {marked} pending finding(s) as notified")
        return 0

    first_run = not state.get("initialized", False)
    report: dict[str, Any] = {
        "config": str(config_path),
        "first_run": first_run,
        "notify_on_first_run": args.notify_on_first_run,
        "source_results": [],
        "manifest_results": [],
        "probe_results": [],
        "errors": [],
    }

    all_findings: list[dict[str, Any]] = []
    for source in config.get("sources", []):
        try:
            if source["type"] == "feed":
                findings, source_report = scan_feed(config, source, args.timeout)
            elif source["type"] == "github_api_versions":
                findings, source_report = scan_github_api_versions(source, args.timeout)
            elif source["type"] == "page_contains":
                findings, source_report = scan_page_contains(source, args.timeout)
            elif source["type"] == "page_terms":
                findings, source_report = scan_page_terms(source, args.timeout)
            elif source["type"] == "reference_page":
                findings, source_report = scan_reference_page(source, args.timeout)
            elif source["type"] == "eol_api":
                findings, source_report = scan_eol_api(source, args.timeout)
            else:
                raise ValueError(f"unsupported source type: {source['type']}")
            source_report.update({"id": source["id"], "name": source["name"], "type": source["type"]})
            report["source_results"].append(source_report)
            all_findings.extend(findings)
        except Exception as exc:  # noqa: BLE001 - report and continue across independent sources
            report["errors"].append({"id": source.get("id"), "url": source.get("url"), "error": str(exc)})

    for scan in config.get("manifest_scans", []):
        try:
            findings, manifest_report = scan_kubernetes_api_versions(scan)
            manifest_report.update({"id": scan["id"], "name": scan["name"], "type": scan["type"]})
            report["manifest_results"].append(manifest_report)
            all_findings.extend(findings)
        except Exception as exc:  # noqa: BLE001 - report and continue across independent manifest scans
            report["errors"].append({"id": scan.get("id"), "url": "local manifests", "error": str(exc)})

    for probe in config.get("header_probes", []):
        try:
            findings, probe_report = scan_header_probe(probe, args.timeout)
            probe_report.update({"id": probe["id"], "name": probe["name"], "type": "header_probe"})
            report["probe_results"].append(probe_report)
            all_findings.extend(findings)
        except Exception as exc:  # noqa: BLE001 - report and continue across independent probes
            report["errors"].append({"id": probe.get("id"), "url": probe.get("url"), "error": str(exc)})

    checked_ids = {str(source.get("id")) for source in config.get("sources", [])}
    checked_ids.update(str(scan.get("id")) for scan in config.get("manifest_scans", []))
    checked_ids.update(str(probe.get("id")) for probe in config.get("header_probes", []))

    new_findings = update_state_and_filter(
        state,
        all_findings,
        first_run=first_run,
        notify_on_first_run=args.notify_on_first_run,
    )
    source_error_findings = source_health_findings(state, report["errors"], checked_ids)
    new_error_findings = update_state_and_filter(
        state,
        source_error_findings,
        first_run=False,
        notify_on_first_run=True,
    )
    new_findings.extend(new_error_findings)
    trim_state(state)

    report.update(
        {
            "total_matches": len(all_findings),
            "new_findings": len(new_findings),
            "new_source_errors": len(new_error_findings),
            "baselined_findings": len(all_findings) if first_run and not args.notify_on_first_run else 0,
            "findings": new_findings,
            "pending_notifications": len(pending_findings(state)),
        }
    )
    write_json(state_out_path, state)
    write_json(report_path, report)

    current_pending = pending_findings(state)
    write_slack_payloads(slack_payload_path, current_pending, first_run, len(all_findings))

    summary = textwrap.dedent(
        f"""
        External compatibility scan complete.
        first_run={first_run}
        total_matches={len(all_findings)}
        new_findings={len(new_findings)}
        errors={len(report['errors'])}
        state={state_out_path}
        report={report_path}
        """
    ).strip()
    print(summary)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
