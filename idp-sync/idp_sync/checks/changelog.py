# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Check 3 — breaking changes del CHANGELOG, **incluyendo las minas de releases anteriores**.

Leer solo la seccion del release nuevo no alcanza y ya nos paso: las Git secret APIs
se anunciaron como deprecadas en la seccion de **v1.1.0**
(«deprecated and removed in v1.2») y se removieron en v1.2 **sin una sola linea en la
seccion de v1.2**. Quien leyera solo v1.2 no veia nada.

Entonces el check hace tres pasadas:

1. **Rango (base, target]** — todas las secciones intermedias, no solo la del tag nuevo.
   Un salto de v1.2.2 a v1.4.0 arrastra los breaking de v1.3.0.
2. **Minas** — sobre TODAS las secciones (incluidas las que ya corremos), busca anuncios
   del tipo «removed in vX.Y» y marca los que **se materializan en este bump**.
3. **Removals sueltos** — bullets con verbo de remocion que NO estan bajo
   «Breaking Changes». El upstream no siempre los clasifica.
"""

from __future__ import annotations

import re
from collections.abc import Iterable
from typing import Any, NamedTuple

from ..model import CheckResult, Finding, Severity
from ..util import Git

CHECK_ID = "changelog"
ORDER = 3
TITLE = "Breaking changes del CHANGELOG"

CHANGELOG_PATH = "CHANGELOG.md"

BREAKING_HEADINGS = ("breaking",)
DEPRECATION_HEADINGS = ("deprecat",)

REMOVAL_ANNOUNCEMENT = re.compile(
    r"\b(?:removed|removal|remove|dropped|drops|drop|deleted|hidden|hide)\s+in\s+(v?\d+\.\d+(?:\.\d+)?)",
    re.IGNORECASE,
)
REMOVAL_VERB = re.compile(
    r"\b(?:removed|no longer|dropped|deleted|replaced by|renamed to|must (?:now )?use)\b",
    re.IGNORECASE,
)


class Version(NamedTuple):
    major: int
    minor: int
    patch: int
    is_release: int  # 1 = release final, 0 = prerelease. Ordena rc/m/alpha ANTES del final.
    pre: str

    def short(self) -> str:
        base = f"v{self.major}.{self.minor}.{self.patch}"
        return base if self.is_release else f"{base}-{self.pre}"


_VERSION_RE = re.compile(r"^v?(\d+)\.(\d+)(?:\.(\d+))?(?:[-.]?(.*))?$")


def parse_version(text: str) -> Version | None:
    match = _VERSION_RE.match(text.strip())
    if not match:
        return None
    major, minor, patch, pre = match.groups()
    pre = (pre or "").strip()
    return Version(int(major), int(minor), int(patch or 0), 0 if pre else 1, pre)


class Section(NamedTuple):
    version: Version
    raw_title: str
    bullets: list[tuple[str, str]]  # (heading, texto del bullet)


def parse_changelog(text: str) -> list[Section]:
    """Parsea `## <version>` / `### <heading>` / bullets `- …` (multilinea)."""
    sections: list[Section] = []
    version: Version | None = None
    raw_title = ""
    heading = ""
    bullets: list[tuple[str, str]] = []
    current: list[str] | None = None

    def flush_bullet() -> None:
        nonlocal current
        if current:
            bullets.append((heading, " ".join(part.strip() for part in current).strip()))
        current = None

    def flush_section() -> None:
        flush_bullet()
        if version is not None:
            sections.append(Section(version, raw_title, list(bullets)))

    for line in text.splitlines():
        if line.startswith("## "):
            flush_section()
            raw_title = line[3:].strip()
            version = parse_version(raw_title)
            heading = ""
            bullets = []
            continue
        if line.startswith("### "):
            flush_bullet()
            heading = line[4:].strip()
            continue
        if line.lstrip().startswith("- "):
            flush_bullet()
            current = [line.lstrip()[2:]]
            continue
        if current is not None and line.strip() and line.startswith((" ", "\t")):
            current.append(line)
            continue
        flush_bullet()

    flush_section()
    return sections


def _heading_is(heading: str, needles: Iterable[str]) -> bool:
    lowered = heading.lower()
    return any(needle in lowered for needle in needles)


def _announced_versions(bullet: str) -> list[Version]:
    found = []
    for raw in REMOVAL_ANNOUNCEMENT.findall(bullet):
        version = parse_version(raw)
        if version:
            found.append(version)
    return found


def analyze(sections: list[Section], base: Version, target: Version) -> list[Finding]:
    findings: list[Finding] = []
    in_range = [s for s in sections if base < s.version <= target]

    if not any(s.version == target for s in sections):
        findings.append(
            Finding(
                severity=Severity.AMARILLO,
                title=f"El CHANGELOG no tiene seccion para `{target.short()}`",
                detail=(
                    "No se puede evaluar el release desde el changelog. El upstream a veces "
                    "publica el tag antes que la entrada del changelog."
                ),
                remediation=(
                    "Leer las release notes de GitHub y el diff de `api/` a mano antes de mergear. "
                    "No tratar el verde de este check como senal."
                ),
            )
        )

    for section in in_range:
        for heading, bullet in section.bullets:
            if _heading_is(heading, BREAKING_HEADINGS):
                findings.append(
                    Finding(
                        severity=Severity.ROJO,
                        title=f"Breaking change en {section.raw_title}",
                        detail=bullet,
                        remediation=(
                            "Evaluar impacto sobre los CRs y values de `idp-platform` antes del bump."
                        ),
                        refs=(f"CHANGELOG.md § {section.raw_title} → {heading}",),
                    )
                )
            elif _heading_is(heading, DEPRECATION_HEADINGS):
                announced = _announced_versions(bullet)
                future = [v for v in announced if v > target]
                findings.append(
                    Finding(
                        severity=Severity.AMARILLO,
                        title=f"Deprecacion anunciada en {section.raw_title}",
                        detail=bullet,
                        remediation=(
                            f"Programar la migracion ANTES de {min(future).short()}: "
                            "cuando llegue ese release el check lo va a marcar en rojo."
                            if future
                            else "La deprecacion no dice en que version se remueve. "
                            "Preguntar upstream y anotarlo, o queda como mina sin fecha."
                        ),
                        refs=(f"CHANGELOG.md § {section.raw_title} → {heading}",),
                    )
                )
            elif REMOVAL_VERB.search(bullet):
                findings.append(
                    Finding(
                        severity=Severity.AMARILLO,
                        title=f"Remocion fuera de «Breaking Changes» ({section.raw_title} → {heading})",
                        detail=bullet,
                        remediation=("El upstream no clasifica todo lo que rompe. Verificar si nos toca."),
                        refs=(f"CHANGELOG.md § {section.raw_title} → {heading}",),
                    )
                )

    # Pasada 2: minas plantadas en releases que YA corremos y que explotan en este bump.
    for section in sections:
        if section.version > base:
            continue
        for heading, bullet in section.bullets:
            for announced in _announced_versions(bullet):
                if base < announced <= target:
                    findings.append(
                        Finding(
                            severity=Severity.ROJO,
                            title=(
                                f"Deprecacion de {section.raw_title} que SE MATERIALIZA "
                                f"en este bump (anunciada para {announced.short()})"
                            ),
                            detail=(
                                bullet + "\n\nEste anuncio esta en una seccion vieja del changelog. La "
                                "seccion del release nuevo puede no mencionarlo — ya paso con las "
                                "Git secret APIs entre v1.1 y v1.2."
                            ),
                            remediation=(
                                "Confirmar en el codigo del tag nuevo que efectivamente se "
                                "removio, y migrar antes de aplicar el bump."
                            ),
                            refs=(f"CHANGELOG.md § {section.raw_title} → {heading}",),
                        )
                    )

    return findings


def run(git: Git, config: dict[str, Any], base_ref: str, target_ref: str) -> CheckResult:
    result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)

    text = git.read_file(target_ref, CHANGELOG_PATH)
    if text is None:
        result.skip(f"No hay `{CHANGELOG_PATH}` en `{target_ref}`.")
        return result

    base_version = parse_version(base_ref)
    target_version = parse_version(target_ref)
    if base_version is None or target_version is None:
        result.skip(
            f"No se pudo interpretar `{base_ref}` / `{target_ref}` como version semver; "
            "el analisis del changelog necesita tags de release."
        )
        return result

    sections = parse_changelog(text)
    for finding in analyze(sections, base_version, target_version):
        result.add(finding)

    in_range = [s.raw_title for s in sections if base_version < s.version <= target_version]
    result.stats = {"sections_parsed": len(sections), "sections_in_range": in_range}
    result.summary = (
        f"Releases analizados: {', '.join(in_range) or '(ninguno)'} "
        f"— mas {len(sections)} secciones revisadas en busca de deprecaciones que venzan ahora."
    )
    return result
