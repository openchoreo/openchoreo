# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Check 4 — colision entre lo que parcheamos y lo que movio el upstream.

Un parche que colisiona no es solo un conflicto de merge: es un fix de seguridad que
puede quedar **silenciosamente revertido** si alguien resuelve el conflicto "tomando el
de upstream". Los dos parches CRITICAL del brief viven exactamente ahi.

El check cruza tres fuentes de "archivos nuestros" — no confia en ninguna sola:

- el **diff real del fork** (`git diff <tag>...main`), que es la verdad;
- los commits con prefijo `PATCH:`, que es la convencion;
- la tabla de **`PATCHES.md`**, que es la documentacion.

Las discrepancias entre las tres son hallazgos por si mismas: `PATCHES.md` dice
«si un archivo del upstream cambio y no esta en la tabla, es un bug».
"""

from __future__ import annotations

import re
from typing import Any

from ..model import CheckResult, Finding, Severity
from ..util import Git, match_glob

CHECK_ID = "patches"
ORDER = 4
TITLE = "Colisiones con nuestros parches"

PATCHES_PATH = "PATCHES.md"
ACTIVE_SECTION = "Parches vigentes"
PATCH_SUBJECT = re.compile(r"^PATCH:", re.IGNORECASE)
BACKTICKED = re.compile(r"`([^`]+)`")


def parse_patches_md(text: str) -> tuple[set[str], list[str]]:
    """Devuelve `(archivos declarados, filas de la tabla de parches vigentes)`.

    Solo mira la seccion «Parches vigentes» (los planificados todavia no tocan nada) y,
    dentro de ella, **solo la columna «Archivos»**. Leer los backticks de toda la fila
    parece mas tolerante y es peor: la columna «Motivo» tiene prosa con backticks, y un
    `.github/` mencionado al pasar termina cubriendo todo `.github/` como si estuviera
    declarado. Eso apaga el hallazgo justo donde tendria que sonar.
    """
    declared: set[str] = set()
    rows: list[str] = []
    in_section = False
    files_column: int | None = None
    header_seen = False

    for line in text.splitlines():
        if line.startswith("## "):
            in_section = ACTIVE_SECTION.lower() in line.lower()
            files_column, header_seen = None, False
            continue
        stripped = line.strip()
        if not in_section or not stripped.startswith("|"):
            continue

        cells = [c.strip() for c in stripped.strip("|").split("|")]
        if not cells or all(c and set(c) <= {"-", ":"} for c in cells):
            continue  # fila separadora

        if not header_seen:
            header_seen = True
            files_column = next((i for i, h in enumerate(cells) if "archivo" in h.lower()), None)
            continue

        rows.append(stripped)
        if files_column is None or files_column >= len(cells):
            continue
        for token in BACKTICKED.findall(cells[files_column]):
            token = token.strip()
            if "/" in token or token.endswith((".go", ".py", ".yaml", ".yml", ".md", ".json")):
                declared.add(token.lstrip("/"))
    return declared, rows


def _covers(declared: set[str], path: str) -> bool:
    """Un directorio declarado (`internal/cluster-gateway/`) cubre sus archivos."""
    for entry in declared:
        if entry == path:
            return True
        if entry.endswith("/") and path.startswith(entry):
            return True
    return False


def run(
    git: Git,
    config: dict[str, Any],
    base_ref: str,
    target_ref: str,
    our_ref: str = "HEAD",
) -> CheckResult:
    result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)
    owned: list[str] = config.get("owned_paths", [])

    fork_files = set(git.fork_diff_files(base_ref, our_ref))

    patch_commit_files: set[str] = set()
    patch_commits: list[tuple[str, str]] = []
    for sha, subject in git.log_subjects(base_ref, our_ref):
        if PATCH_SUBJECT.match(subject):
            patch_commits.append((sha, subject))
            patch_commit_files.update(git.files_of_commit(sha))

    patches_md = git.read_file(our_ref, PATCHES_PATH) or ""
    declared, rows = parse_patches_md(patches_md)

    upstream_changed = set(git.changed_files(base_ref, target_ref))

    # Un archivo nuestro que el upstream tambien movio: hay que resolver a mano.
    ours = fork_files | patch_commit_files
    collisions = sorted(f for f in ours if f in upstream_changed)

    for path in collisions:
        documented = _covers(declared, path)
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"Colision: upstream cambio `{path}`, que nosotros parcheamos",
                detail=(
                    f"El archivo aparece en el diff del fork y cambio "
                    f"entre `{base_ref}` y `{target_ref}`."
                    + (
                        ""
                        if documented
                        else " Ademas **no esta declarado** en la tabla de parches vigentes de `PATCHES.md`."
                    )
                ),
                remediation=(
                    "Rebasear el parche a mano y **verificar que el fix siga aplicado** "
                    "despues del merge: resolver 'tomando el de upstream' revierte el parche "
                    "sin dejar rastro. Dejar constancia en `PATCHES.md`."
                ),
                refs=(path,),
            )
        )

    undocumented = sorted(
        path
        for path in ours
        if not _covers(declared, path) and not match_glob(path, owned) and path != PATCHES_PATH
    )
    for path in undocumented:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"Diff con el upstream sin fila en `PATCHES.md`: `{path}`",
                detail=(
                    "`PATCHES.md` dice: «si un archivo del upstream cambio y no esta en la tabla, "
                    "es un bug». O falta la fila, o el cambio no debio existir."
                ),
                remediation=(
                    "Agregar la fila (parche, archivos, motivo, issue upstream, condicion de "
                    "retiro) o revertir el cambio. Si es un archivo nuestro por construccion, "
                    "sumarlo a `owned_paths` en `idp-sync/upstream.json`."
                ),
                refs=(path,),
            )
        )

    if not rows and (fork_files - set(owned)):
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title="`PATCHES.md` no tiene parches vigentes pero el fork difiere del upstream",
                detail="La tabla de «Parches vigentes» esta vacia y sin embargo hay archivos con diff.",
                remediation="Completar la tabla antes del bump.",
            )
        )

    result.stats = {
        "fork_diff_files": len(fork_files),
        "patch_commits": len(patch_commits),
        "declared_in_patches_md": sorted(declared),
        "collisions": collisions,
    }
    result.summary = (
        f"{len(collisions)} colision(es) sobre {len(ours)} archivo(s) con diff propio; "
        f"{len(patch_commits)} commit(s) `PATCH:`."
    )
    return result
