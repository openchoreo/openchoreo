# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Check 5 — `helm template` con NUESTROS values, antes y despues, y diff del render.

Los checks 1-4 miran declaraciones; este mira el resultado. Es el unico que puede
contestar «que cambia realmente en el cluster», que es la pregunta.

Dos decisiones de diseno:

- Se renderiza **`base_tag` vs `target_tag`**, no `main` vs `target_tag`. Comparar contra
  `main` mezclaria el delta del upstream con el de nuestros propios parches y el diff se
  vuelve ilegible. Que archivos parcheados movio el upstream ya lo contesta el check 4.
- Si no hay values de `idp-platform`, se renderiza con los defaults del chart y el check
  queda en **amarillo**: un render con defaults no representa produccion, y decir verde
  ahi seria mentir.
"""

from __future__ import annotations

import difflib
import pathlib
import re
import subprocess
import tempfile
from typing import Any

from ..model import CheckResult, Finding, Severity
from ..util import Git, load_yaml_documents, truncate

CHECK_ID = "helm-render"
ORDER = 5
TITLE = "Diff del `helm template`"

RELEASE_NAME = "idp"
NAMESPACE = "idp-control-plane"
HELM_TIMEOUT = 300


def _helm(args: list[str], cwd: pathlib.Path | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["helm", *args],
        cwd=str(cwd) if cwd else None,
        capture_output=True,
        text=True,
        timeout=HELM_TIMEOUT,
    )


def render_chart(chart_dir: pathlib.Path, values_file: pathlib.Path | None) -> tuple[str | None, str]:
    """Devuelve `(manifiesto, error)`. `manifiesto is None` significa que fallo el render."""
    if (chart_dir / "Chart.lock").is_file() or (chart_dir / "charts").is_dir():
        dep = _helm(["dependency", "build", str(chart_dir)])
        if dep.returncode != 0:
            # No es fatal: puede no haber red. Se intenta el template igual.
            pass

    args = ["template", RELEASE_NAME, str(chart_dir), "--namespace", NAMESPACE]
    if values_file is not None:
        args += ["-f", str(values_file)]
    proc = _helm(args)
    if proc.returncode != 0:
        return None, proc.stderr.strip()
    return proc.stdout, ""


def resource_keys(manifest: str) -> set[str]:
    """`kind/namespace/name` de cada documento renderizado."""
    keys: set[str] = set()
    for doc in load_yaml_documents(manifest):
        if not isinstance(doc, dict):
            continue
        kind = doc.get("kind")
        meta = doc.get("metadata") or {}
        if kind:
            keys.add(f"{kind}/{meta.get('namespace', '-')}/{meta.get('name', '?')}")
    return keys


def diff_manifests(old: str, new: str, chart: str) -> str:
    return "\n".join(
        difflib.unified_diff(
            old.splitlines(),
            new.splitlines(),
            fromfile=f"{chart}@base",
            tofile=f"{chart}@target",
            lineterm="",
            n=2,
        )
    )


def _sensitive_hits(diff_text: str, config: dict[str, Any]) -> list[str]:
    render_cfg = config.get("helm_render", {})
    needles = list(render_cfg.get("sensitive_kinds", [])) + list(render_cfg.get("sensitive_fields", []))
    changed = [
        line
        for line in diff_text.splitlines()
        if line.startswith(("+", "-")) and not line.startswith(("+++", "---"))
    ]
    hits = []
    for needle in needles:
        pattern = re.compile(rf"\b{re.escape(needle)}\b")
        if any(pattern.search(line) for line in changed):
            hits.append(needle)
    return hits


def run(
    git: Git,
    config: dict[str, Any],
    base_ref: str,
    target_ref: str,
    platform_root: pathlib.Path | None = None,
    platform_missing_reason: str = "",
) -> CheckResult:
    result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)

    if subprocess.run(["which", "helm"], capture_output=True).returncode != 0:
        result.skip("`helm` no esta instalado en el runner.")
        return result

    values_map: dict[str, str] = config.get("platform", {}).get("values", {})
    if platform_root is None:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title="Sin `IDP_PLATFORM_TOKEN`: render con defaults, no con nuestros values",
                detail=(
                    "No hubo checkout de `idp-platform`. El diff que sigue es del chart pelado y "
                    "**no representa lo que se va a desplegar**.\n\n"
                    + (
                        platform_missing_reason
                        or "Falta configurar el secret `IDP_PLATFORM_TOKEN` con lectura sobre "
                        "`fondomp-production/idp-platform`."
                    )
                ),
                remediation=(
                    "Configurar el secret `IDP_PLATFORM_TOKEN` con lectura sobre "
                    "`fondomp-production/idp-platform`. Hasta entonces este check debe quedar "
                    "amarillo, nunca verde."
                ),
            )
        )

    with tempfile.TemporaryDirectory(prefix="idp-sync-helm-") as tmp:
        tmp_path = pathlib.Path(tmp)
        base_tree = tmp_path / "base"
        target_tree = tmp_path / "target"
        git.add_worktree(base_ref, base_tree)
        git.add_worktree(target_ref, target_tree)
        try:
            charts_changed = 0
            for chart in config["charts"]:
                name = chart.split("/")[-1]
                values_file = None
                rel = values_map.get(chart)
                if platform_root is not None and rel and (platform_root / rel).is_file():
                    values_file = platform_root / rel
                elif platform_root is not None and rel:
                    result.add(
                        Finding(
                            severity=Severity.AMARILLO,
                            title=f"[{name}] faltan nuestros values (`{rel}`)",
                            detail=(
                                "El repo `idp-platform` no tiene ese archivo todavia; se renderiza con "
                                "defaults."
                            ),
                            remediation=(
                                "Crear el values en `idp-platform` (track T10) o corregir la ruta en "
                                "`idp-sync/upstream.json`."
                            ),
                        )
                    )

                old_manifest, old_err = render_chart(base_tree / chart, values_file)
                new_manifest, new_err = render_chart(target_tree / chart, values_file)

                # Que falle el render solo es ROJO si es una **regresion**: antes andaba y
                # ahora no. Si falla en los dos refs, el bump no lo causo — y con los values
                # por default falla siempre (los charts exigen hostnames reales y secretName).
                # Marcarlo rojo cada corrida seria un rojo permanente que nadie mira.
                if new_manifest is None and old_manifest is None:
                    result.add(
                        Finding(
                            severity=Severity.AMARILLO,
                            title=f"[{name}] no renderiza en NINGUNO de los dos refs",
                            detail=(
                                f"Mismo error antes y despues, asi que no lo introduce el bump.\n\n"
                                f"```\n{truncate(new_err, 30)}\n```"
                            ),
                            remediation=(
                                "Con values propios: arreglarlos, hoy el chart no renderiza. "
                                "Sin values propios: es esperable —los charts exigen hostnames "
                                "reales y `secretName`— y el check no puede dar senal hasta que "
                                "exista el values en `idp-platform` (track T10)."
                            ),
                        )
                    )
                    continue
                if new_manifest is None:
                    result.add(
                        Finding(
                            severity=Severity.ROJO,
                            title=f"[{name}] el chart nuevo NO renderiza y el viejo si",
                            detail=f"Regresion introducida por el bump.\n\n```\n{truncate(new_err, 40)}\n```",
                            remediation=(
                                "Ajustar los values antes del bump; el `helm upgrade` va a fallar igual."
                            ),
                        )
                    )
                    continue
                if old_manifest is None:
                    result.add(
                        Finding(
                            severity=Severity.AMARILLO,
                            title=f"[{name}] el chart viejo no renderiza y el nuevo si",
                            detail=f"```\n{truncate(old_err, 20)}\n```",
                            remediation=(
                                "El bump arregla el render. Verificar que sea intencional y no un default "
                                "nuevo que nos tapa un error."
                            ),
                        )
                    )
                    continue

                if old_manifest == new_manifest:
                    continue
                charts_changed += 1

                old_keys, new_keys = resource_keys(old_manifest), resource_keys(new_manifest)
                for key in sorted(old_keys - new_keys):
                    result.add(
                        Finding(
                            severity=Severity.ROJO,
                            title=f"[{name}] deja de renderizarse `{key}`",
                            detail=(
                                "El recurso desaparece del release. `helm upgrade` lo borra del cluster."
                            ),
                            remediation=(
                                "Confirmar que es intencional. Si algo depende de ese objeto, se cae en el "
                                "upgrade."
                            ),
                        )
                    )
                for key in sorted(new_keys - old_keys):
                    result.add(
                        Finding(
                            severity=Severity.AMARILLO,
                            title=f"[{name}] recurso nuevo `{key}`",
                            detail="Aparece en el render.",
                            remediation="Verificar que no choque con algo que ya gestionamos por GitOps.",
                        )
                    )

                diff_text = diff_manifests(old_manifest, new_manifest, name)
                hits = _sensitive_hits(diff_text, config)
                if hits:
                    result.add(
                        Finding(
                            severity=Severity.AMARILLO,
                            title=f"[{name}] el diff toca superficie sensible: {', '.join(sorted(hits))}",
                            detail=f"```diff\n{truncate(diff_text)}\n```",
                            remediation=(
                                "Leer el diff completo. Los dos parches CRITICAL del brief viven "
                                "en RBAC "
                                "y en el cluster-gateway: un cambio de ClusterRole del upstream "
                                "puede "
                                "reintroducir el comodin."
                            ),
                        )
                    )
                elif not any(f.severity is Severity.ROJO for f in result.findings):
                    result.add(
                        Finding(
                            severity=Severity.AMARILLO,
                            title=f"[{name}] el render cambia",
                            detail=f"```diff\n{truncate(diff_text)}\n```",
                            remediation="Revisar el diff.",
                        )
                    )
        finally:
            git.remove_worktree(base_tree)
            git.remove_worktree(target_tree)

    result.stats["charts_with_render_changes"] = charts_changed
    result.summary = (
        f"{charts_changed} de {len(config['charts'])} charts cambian el render."
        if charts_changed
        else "El render de los 4 charts es identico."
    )
    return result
