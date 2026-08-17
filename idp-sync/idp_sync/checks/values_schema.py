# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Check 2 — diff de `values.schema.json` de los 4 charts.

La trampa: el efecto de sacar una clave depende de `additionalProperties` del
objeto que la contiene.

- `additionalProperties: false` → helm **falla ruidosamente**. Molesto, pero visible.
- sin esa restriccion → helm **acepta el value y lo ignora**. El chart renderiza con el
  default del upstream y nadie se entera. Este es el modo de falla que importa.

Por eso el check distingue los dos casos, y sube a ROJO cuando la clave removida es
una que NOSOTROS seteamos en `idp-platform`.
"""

from __future__ import annotations

import json
from typing import Any

from ..model import CheckResult, Finding, Severity
from ..util import Git, load_yaml

CHECK_ID = "values-schema"
ORDER = 2
TITLE = "Diff de `values.schema.json`"


def index_schema(node: Any, path: str = "") -> dict[str, dict[str, Any]]:
    """Aplana el JSON Schema a `{ruta.dotted: nodo}` siguiendo `properties`."""
    index: dict[str, dict[str, Any]] = {}
    if not isinstance(node, dict):
        return index
    if path:
        index[path] = node
    for name, child in (node.get("properties") or {}).items():
        index.update(index_schema(child, f"{path}.{name}" if path else name))
    items = node.get("items")
    if isinstance(items, dict):
        index.update(index_schema(items, f"{path}[]"))
    return index


def flatten_values(values: Any, path: str = "") -> set[str]:
    """Rutas dotted que un `values.yaml` setea explicitamente."""
    paths: set[str] = set()
    if not isinstance(values, dict):
        return paths
    for name, child in values.items():
        current = f"{path}.{name}" if path else str(name)
        paths.add(current)
        paths |= flatten_values(child, current)
    return paths


def _parent_path(path: str) -> str:
    return path.rsplit(".", 1)[0] if "." in path else ""


def _closed(index: dict[str, dict[str, Any]], root: dict[str, Any], path: str) -> bool:
    """True si el objeto padre declara `additionalProperties: false` (falla ruidosa)."""
    parent = _parent_path(path)
    node = root if not parent else index.get(parent, {})
    return node.get("additionalProperties") is False


def _compare(
    result: CheckResult,
    chart: str,
    old_schema: dict[str, Any],
    new_schema: dict[str, Any],
    our_values: set[str] | None,
) -> None:
    old_index = index_schema(old_schema)
    new_index = index_schema(new_schema)

    for path in sorted(set(old_index) - set(new_index)):
        we_set_it = our_values is not None and path in our_values
        loud = _closed(old_index, old_schema, path)
        if we_set_it:
            severity = Severity.ROJO
            detail = (
                f"`{path}` desaparece del schema y **nosotros la seteamos** en los values de `idp-platform`."
            )
        elif our_values is None:
            severity = Severity.AMARILLO
            detail = (
                f"`{path}` desaparece del schema. No se pudo cruzar contra nuestros values "
                "(no habia checkout de `idp-platform`), asi que no se descarta que la usemos."
            )
        else:
            severity = Severity.AMARILLO
            detail = f"`{path}` desaparece del schema. Hoy no la seteamos."
        detail += (
            "\n\nEfecto: helm **falla** al validar (el padre tiene `additionalProperties: false`)."
            if loud
            else "\n\nEfecto: helm **acepta el value y lo ignora en silencio**; "
            "el chart renderiza con el default del upstream."
        )
        result.add(
            Finding(
                severity=severity,
                title=f"[{chart}] clave removida `{path}`",
                detail=detail,
                remediation=(
                    "Buscar la clave en los values de `idp-platform` y migrarla al reemplazo que "
                    "indique el CHANGELOG. Si no hay reemplazo, confirmar que el comportamiento "
                    "por default es el que queremos."
                ),
            )
        )

    for path in sorted(set(old_index) & set(new_index)):
        old_node, new_node = old_index[path], new_index[path]

        if old_node.get("type") != new_node.get("type"):
            result.add(
                Finding(
                    severity=Severity.ROJO,
                    title=f"[{chart}] `{path}` cambio de tipo",
                    detail=f"`type`: {old_node.get('type')!r} → {new_node.get('type')!r}",
                    remediation="Ajustar el value en `idp-platform` en el mismo PR del bump.",
                )
            )

        old_enum, new_enum = old_node.get("enum"), new_node.get("enum")
        if old_enum and new_enum:
            dropped = [v for v in old_enum if v not in new_enum]
            if dropped:
                result.add(
                    Finding(
                        severity=Severity.ROJO,
                        title=f"[{chart}] `{path}` perdio valores de enum",
                        detail=f"Ya no se aceptan: {dropped}",
                        remediation="Si alguno es el que usamos, el `helm upgrade` va a fallar.",
                    )
                )

        if old_node.get("default") != new_node.get("default"):
            result.add(
                Finding(
                    severity=Severity.AMARILLO,
                    title=f"[{chart}] cambio el default de `{path}`",
                    detail=f"{old_node.get('default')!r} → {new_node.get('default')!r}",
                    remediation=(
                        "Si no seteamos la clave explicitamente, el comportamiento cambia solo. "
                        "Considerar fijarla en los values."
                    ),
                )
            )

        old_ap, new_ap = old_node.get("additionalProperties"), new_node.get("additionalProperties")
        if old_ap is not False and new_ap is False:
            result.add(
                Finding(
                    severity=Severity.AMARILLO,
                    title=f"[{chart}] `{path or '<root>'}` pasa a ser cerrado",
                    detail=(
                        "`additionalProperties: false`: cualquier clave extra que pasemos "
                        "hace fallar el render."
                    ),
                    remediation="Correr `helm template` con nuestros values (check 5) antes de mergear.",
                )
            )

    for path in sorted(set(new_index) - set(old_index)):
        parent = _parent_path(path)
        parent_node = new_schema if not parent else new_index.get(parent, {})
        if path.rsplit(".", 1)[-1] in (parent_node.get("required") or []):
            result.add(
                Finding(
                    severity=Severity.ROJO,
                    title=f"[{chart}] clave nueva OBLIGATORIA `{path}`",
                    detail="El chart no renderiza sin ella.",
                    remediation="Agregarla a los values de `idp-platform` en el mismo PR del bump.",
                )
            )

    old_required = set(old_schema.get("required") or [])
    new_required = set(new_schema.get("required") or [])
    for key in sorted(new_required - old_required):
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"[{chart}] `{key}` pasa a ser obligatorio en la raiz",
                detail="Sin esa clave el chart no renderiza.",
                remediation="Agregarla a los values de `idp-platform`.",
            )
        )


def run(
    git: Git,
    config: dict[str, Any],
    base_ref: str,
    target_ref: str,
    platform_root: Any = None,
    platform_missing_reason: str = "",
) -> CheckResult:
    result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)
    values_map: dict[str, str] = config.get("platform", {}).get("values", {})

    if platform_root is None:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title="Sin `IDP_PLATFORM_TOKEN`: no se pudo leer `idp-platform`",
                detail=(
                    "El check corre igual sobre el schema, pero no puede cruzar contra nuestros "
                    "values. Por eso no puede decidir si una clave removida es una que usamos; "
                    "todas quedan en amarillo por las dudas.\n\n"
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

    changed_charts = 0
    for chart in config["charts"]:
        schema_path = f"{chart}/values.schema.json"
        old_raw = git.read_file(base_ref, schema_path)
        new_raw = git.read_file(target_ref, schema_path)

        if old_raw is None and new_raw is None:
            continue
        if old_raw is None:
            result.add(
                Finding(
                    severity=Severity.AMARILLO,
                    title=f"[{chart}] `values.schema.json` es nuevo",
                    detail="El chart empieza a validar values que antes pasaban sin control.",
                    remediation="Correr `helm template` con nuestros values (check 5).",
                )
            )
            continue
        if new_raw is None:
            result.add(
                Finding(
                    severity=Severity.AMARILLO,
                    title=f"[{chart}] desaparecio `values.schema.json`",
                    detail="Se pierde la validacion de values: los typos dejan de dar error.",
                    remediation="Compensar con validacion propia (Conftest sobre los values).",
                )
            )
            continue
        if old_raw == new_raw:
            continue

        changed_charts += 1
        our_values: set[str] | None = None
        rel = values_map.get(chart)
        if platform_root is not None and rel:
            values_file = platform_root / rel
            if values_file.is_file():
                our_values = flatten_values(load_yaml(values_file.read_text(encoding="utf-8")))

        _compare(result, chart.split("/")[-1], json.loads(old_raw), json.loads(new_raw), our_values)

    result.stats["charts_with_schema_changes"] = changed_charts
    result.summary = (
        f"{changed_charts} de {len(config['charts'])} charts cambiaron su `values.schema.json`."
        if changed_charts
        else "Ningun `values.schema.json` cambio."
    )
    return result
