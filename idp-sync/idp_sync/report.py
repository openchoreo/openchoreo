# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Orquestacion de los checks y verificaciones previas del pin.

Los checks corren en dos grupos porque tienen costos muy distintos:

- **estatico** (1-5): minutos, sin cluster. Es el grueso de la senal.
- **k3d** (6): decenas de minutos, con cluster. Corre en un job aparte para que su
  fragilidad no se lleve puesto el reporte entero.
"""

from __future__ import annotations

import pathlib
import re
from typing import Any

from .checks import changelog, cr_validation, crds, helm_render, patches, values_schema
from .model import CheckResult, Finding, Report, Severity
from .util import Git

PIN_CHECK_ID = "pin"
PIN_ORDER = 0
PIN_TITLE = "Consistencia del pin"

_PATCHES_TAG = re.compile(r"\|\s*Tag fijado\s*\|\s*\*{0,2}`?(v[0-9][^`*|\s]*)`?\*{0,2}\s*\|")
_PATCHES_COMMIT = re.compile(r"\|\s*Commit\s*\|\s*`?([0-9a-f]{7,40})`?\s*\|")


def check_pin(git: Git, config: dict[str, Any], our_ref: str = "HEAD") -> CheckResult:
    """Verifica que `upstream.json`, `PATCHES.md` y el espejo digan lo mismo.

    Si estas tres fuentes divergen, todos los checks siguientes comparan contra el ref
    equivocado y el reporte entero deja de significar algo. Por eso va antes.
    """
    result = CheckResult(check_id=PIN_CHECK_ID, order=PIN_ORDER, title=PIN_TITLE)
    pinned = config["pinned"]
    tag, commit = pinned["tag"], pinned["commit"]

    if not git.ref_exists(tag):
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"El tag fijado `{tag}` no existe localmente",
                detail="Sin el tag no hay base contra la cual diffear.",
                remediation="`git fetch upstream --tags` antes de correr el agente.",
            )
        )
        return result

    actual = git.rev_parse(tag)
    if actual != commit:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"`upstream.json` apunta a un commit que no es el de `{tag}`",
                detail=f"declarado `{commit[:12]}` · real `{actual[:12]}`",
                remediation=(
                    f"Corregir `pinned.commit` con `git rev-parse {tag}^{{commit}}`. "
                    "Ojo: los tags del upstream son **anotados**, asi que `git rev-parse <tag>` "
                    "a secas devuelve el sha del objeto tag, no el del commit."
                ),
            )
        )

    mirror_branch = config["upstream"]["mirror_branch"]
    for candidate in (mirror_branch, f"origin/{mirror_branch}"):
        if git.ref_exists(candidate):
            if git.rev_parse(candidate) != actual:
                result.add(
                    Finding(
                        severity=Severity.AMARILLO,
                        title=f"`{candidate}` no esta parado en `{tag}`",
                        detail=(
                            "La rama espejo tiene que ser un reflejo puro del tag fijado. "
                            "Si avanzo, el diff del fork (check 4) mezcla cambios del upstream "
                            "con los nuestros."
                        ),
                        remediation=f"`git branch -f {mirror_branch} {tag}` (o corregir el pin).",
                    )
                )
            break

    patches_md = git.read_file(our_ref, "PATCHES.md") or ""
    tag_match = _PATCHES_TAG.search(patches_md)
    if tag_match and tag_match.group(1) != tag:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title="`PATCHES.md` y `idp-sync/upstream.json` no coinciden en el tag",
                detail=f"`PATCHES.md`: `{tag_match.group(1)}` · `upstream.json`: `{tag}`",
                remediation=(
                    "Actualizar los dos juntos. `PATCHES.md` es la fuente humana; "
                    "`upstream.json`, la maquinable."
                ),
            )
        )
    commit_match = _PATCHES_COMMIT.search(patches_md)
    if commit_match and not actual.startswith(commit_match.group(1)):
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title="El commit de `PATCHES.md` no es el commit del tag",
                detail=(
                    f"`PATCHES.md`: `{commit_match.group(1)[:12]}` · "
                    f"commit real de `{tag}`: `{actual[:12]}`. "
                    "Suele ser el sha del **objeto tag** anotado, que no es el commit."
                ),
                remediation=f"Reemplazar por la salida de `git rev-parse {tag}^{{commit}}`.",
            )
        )

    result.summary = f"Pin: `{tag}` @ `{actual[:12]}`."
    return result


EXPECTED_CHECKS = (
    (PIN_CHECK_ID, PIN_ORDER, PIN_TITLE),
    (crds.CHECK_ID, crds.ORDER, crds.TITLE),
    (values_schema.CHECK_ID, values_schema.ORDER, values_schema.TITLE),
    (changelog.CHECK_ID, changelog.ORDER, changelog.TITLE),
    (patches.CHECK_ID, patches.ORDER, patches.TITLE),
    (helm_render.CHECK_ID, helm_render.ORDER, helm_render.TITLE),
    (cr_validation.CHECK_ID, cr_validation.ORDER, cr_validation.TITLE),
)


def ensure_complete(report: Report) -> Report:
    """Rellena los checks ausentes como salteados.

    Si el job del k3d no corrio, su artifact no existe y el check desapareceria del
    reporte. Un reporte con cinco filas se lee como "el sexto dio bien"; con la fila en
    amarillo diciendo que no corrio, no.
    """
    present = {result.check_id for result in report.results}
    for check_id, order, title in EXPECTED_CHECKS:
        if check_id in present:
            continue
        missing = CheckResult(check_id=check_id, order=order, title=title)
        missing.skip("El job que produce este check no dejo resultado (fallo, timeout o salteado).")
        report.results.append(missing)
    return report


def run_static(
    git: Git,
    config: dict[str, Any],
    base_ref: str,
    target_ref: str,
    platform_root: pathlib.Path | None,
    platform_missing_reason: str = "",
    our_ref: str = "HEAD",
    skip: tuple[str, ...] = (),
) -> Report:
    report = Report(base_ref=base_ref, target_ref=target_ref)
    report.results.append(check_pin(git, config, our_ref))

    if crds.CHECK_ID not in skip:
        report.results.append(crds.run(git, config, base_ref, target_ref))
    if values_schema.CHECK_ID not in skip:
        report.results.append(
            values_schema.run(git, config, base_ref, target_ref, platform_root, platform_missing_reason)
        )
    if changelog.CHECK_ID not in skip:
        report.results.append(changelog.run(git, config, base_ref, target_ref))
    if patches.CHECK_ID not in skip:
        report.results.append(patches.run(git, config, base_ref, target_ref, our_ref))
    if helm_render.CHECK_ID not in skip:
        report.results.append(
            helm_render.run(git, config, base_ref, target_ref, platform_root, platform_missing_reason)
        )

    return report


def run_k3d(
    git: Git,
    config: dict[str, Any],
    sync_dir: pathlib.Path,
    base_ref: str,
    target_ref: str,
    platform_root: pathlib.Path | None,
    workdir: pathlib.Path,
    platform_missing_reason: str = "",
) -> Report:
    report = Report(base_ref=base_ref, target_ref=target_ref)
    tree = workdir / "target"
    git.add_worktree(target_ref, tree)
    try:
        cr_dirs: list[pathlib.Path] = []
        values_file: pathlib.Path | None = None
        if platform_root is not None:
            cr_dirs = [
                platform_root / d
                for d in config["platform"]["cr_dirs"]
                if (platform_root / d).is_dir() and any((platform_root / d).rglob("*.y*ml"))
            ]
            control_plane = config["charts"][0]
            rel = config["platform"]["values"].get(control_plane)
            if rel and (platform_root / rel).is_file():
                values_file = platform_root / rel

        report.results.append(
            cr_validation.run(
                sync_dir=sync_dir,
                chart_dir=tree / config["charts"][0],
                crd_dir=tree / config["crd_dirs"][0],
                cr_dirs=cr_dirs,
                values_file=values_file,
                out_file=workdir / "k3d-result.json",
                platform_missing_reason=platform_missing_reason if platform_root is None else "",
            )
        )
    finally:
        git.remove_worktree(tree)
    return report
