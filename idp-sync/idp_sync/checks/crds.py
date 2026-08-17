# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Check 1 — diff de los CRDs. La superficie mas peligrosa del bump.

Por que va primero y por que es el unico check que puede tenir de rojo un bump
que "no toca codigo":

1. **No hay conversion webhooks.** Si una version deja de estar servida o de ser
   la de almacenamiento, los objetos guardados con esa version quedan ilegibles.
   No hay migracion automatica.
2. **Helm NUNCA actualiza `crds/` en `upgrade`** (solo en `install`). Un
   `helm upgrade` deja los CRDs viejos en su lugar y el chart nuevo empieza a
   emitir campos que la API server desconoce.
3. **El pruning es silencioso.** Si upstream saca una propiedad y nuestro CR la
   sigue seteando, la API la descarta sin error. El CR queda aplicado, el campo
   no existe, y nadie se entera hasta que falla en runtime.

Por eso: *cualquier* cambio en un CRD es como minimo AMARILLO y arrastra un paso
manual de `kubectl apply --server-side`; un cambio que ACHICA el schema es ROJO.
"""

from __future__ import annotations

import pathlib
from typing import Any

from ..model import CheckResult, Finding, Severity
from ..util import Git, load_yaml

CHECK_ID = "crds"
ORDER = 1
TITLE = "Diff de los CRDs"

APPLY_HINT = (
    "Antes del `helm upgrade`, aplicar los CRDs a mano: "
    "`kubectl apply --server-side --force-conflicts -f config/crd/bases/`. "
    "Helm no actualiza `crds/` en upgrade."
)
BACKUP_HINT = (
    "Backup previo obligatorio de los 37 kinds del namespace del control plane "
    "(`kubectl get <kind> -A -o yaml`): los CRs son el estado autoritativo y los CRDs no se "
    "revierten solos."
)

# Nodos del schema que, si cambian, cambian la forma del dato.
_SHAPE_KEYS = (
    "type",
    "format",
    "default",
    "pattern",
    "maximum",
    "minimum",
    "maxLength",
    "minLength",
)


def _index_schema(node: Any, path: str = "") -> dict[str, dict[str, Any]]:
    """Aplana un openAPIV3Schema a `{ruta: nodo}`.

    Recorre `properties`, `items` y `additionalProperties` como schema. Las rutas
    quedan tipo `spec.template.containers[].image`.
    """
    index: dict[str, dict[str, Any]] = {}
    if not isinstance(node, dict):
        return index

    if path:
        index[path] = node

    for name, child in (node.get("properties") or {}).items():
        index.update(_index_schema(child, f"{path}.{name}" if path else name))

    items = node.get("items")
    if isinstance(items, dict):
        index.update(_index_schema(items, f"{path}[]"))

    extra = node.get("additionalProperties")
    if isinstance(extra, dict):
        index.update(_index_schema(extra, f"{path}{{}}"))

    return index


def _versions(crd: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {v.get("name", "?"): v for v in (crd.get("spec", {}).get("versions") or [])}


def _crd_name(crd: dict[str, Any]) -> str:
    return crd.get("metadata", {}).get("name", "?")


def _compare_version_schemas(
    result: CheckResult,
    crd_name: str,
    version: str,
    old_schema: dict[str, Any],
    new_schema: dict[str, Any],
) -> None:
    old_index = _index_schema(old_schema)
    new_index = _index_schema(new_schema)

    for path in sorted(set(old_index) - set(new_index)):
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"{crd_name} ({version}): propiedad removida `{path}`",
                detail=(
                    "La propiedad desaparece del schema. Si algun CR de `idp-platform` la setea, "
                    "la API la descarta **en silencio** (pruning): el apply sigue devolviendo OK "
                    "y el campo simplemente deja de existir."
                ),
                remediation=(
                    f"Buscar `{path.split('.')[-1].rstrip('[]{}')}` en los CRs de `idp-platform` "
                    "antes del bump y migrar al reemplazo que indique el CHANGELOG. " + APPLY_HINT
                ),
            )
        )

    for path in sorted(set(old_index) & set(new_index)):
        old_node, new_node = old_index[path], new_index[path]

        for key in _SHAPE_KEYS:
            if old_node.get(key) != new_node.get(key):
                severity = Severity.ROJO if key == "type" else Severity.AMARILLO
                result.add(
                    Finding(
                        severity=severity,
                        title=f"{crd_name} ({version}): `{path}` cambio `{key}`",
                        detail=f"`{key}`: {old_node.get(key)!r} → {new_node.get(key)!r}",
                        remediation=(
                            "Revisar los CRs que setean esa ruta. " + APPLY_HINT
                            if severity is Severity.ROJO
                            else "Verificar que el valor nuevo sea el que queremos; los defaults "
                            "del upstream no siempre son los nuestros."
                        ),
                    )
                )

        old_enum, new_enum = old_node.get("enum"), new_node.get("enum")
        if old_enum and new_enum:
            dropped = [v for v in old_enum if v not in new_enum]
            if dropped:
                result.add(
                    Finding(
                        severity=Severity.ROJO,
                        title=f"{crd_name} ({version}): `{path}` perdio valores de enum",
                        detail=f"Valores que ya no se aceptan: {dropped}",
                        remediation=(
                            "Cualquier CR que use uno de esos valores va a ser rechazado en el apply."
                        ),
                    )
                )

        old_req = set(old_node.get("required") or [])
        new_req = set(new_node.get("required") or [])
        for field in sorted(new_req - old_req):
            result.add(
                Finding(
                    severity=Severity.ROJO,
                    title=f"{crd_name} ({version}): `{path or '<root>'}.{field}` pasa a ser obligatorio",
                    detail=(
                        "Los CRs que hoy no lo setean van a ser rechazados en el proximo apply, "
                        "incluido el reconcile del propio control plane."
                    ),
                    remediation=(
                        f"Agregar `{field}` a los CRs afectados de `idp-platform` en el MISMO PR del bump."
                    ),
                )
            )

        old_open = old_node.get("x-kubernetes-preserve-unknown-fields")
        new_open = new_node.get("x-kubernetes-preserve-unknown-fields")
        if old_open and not new_open:
            result.add(
                Finding(
                    severity=Severity.ROJO,
                    title=f"{crd_name} ({version}): `{path}` deja de preservar campos desconocidos",
                    detail=(
                        "`x-kubernetes-preserve-unknown-fields` pasa de true a "
                        f"{new_open!r}: todo lo que no este en el schema se prunea en silencio."
                    ),
                    remediation="Revisar los templates CEL y los CRs que escriben bajo esa ruta.",
                )
            )


def _compare_crd(result: CheckResult, path: str, old_crd: dict[str, Any], new_crd: dict[str, Any]) -> None:
    name = _crd_name(new_crd) or path
    old_versions = _versions(old_crd)
    new_versions = _versions(new_crd)

    for version in sorted(set(old_versions) - set(new_versions)):
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"{name}: version `{version}` removida",
                detail=(
                    "OpenChoreo no despliega conversion webhooks. Los objetos almacenados con esta "
                    "version quedan **ilegibles** — no hay conversion automatica."
                ),
                remediation=(
                    "Migrar todos los objetos a la version nueva ANTES del bump "
                    "(leer + reescribir cada CR). " + BACKUP_HINT
                ),
            )
        )

    for version in sorted(set(new_versions) - set(old_versions)):
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"{name}: version nueva `{version}`",
                detail="Version agregada al CRD.",
                remediation=APPLY_HINT,
            )
        )

    for version in sorted(set(old_versions) & set(new_versions)):
        old_v, new_v = old_versions[version], new_versions[version]

        if old_v.get("served") and not new_v.get("served"):
            result.add(
                Finding(
                    severity=Severity.ROJO,
                    title=f"{name}: version `{version}` deja de estar servida",
                    detail="`served: true → false`. Cualquier cliente que la use empieza a recibir 404.",
                    remediation="Migrar clientes y CRs a una version servida antes del bump.",
                )
            )
        if old_v.get("storage") and not new_v.get("storage"):
            new_storage = next((n for n, v in new_versions.items() if v.get("storage")), "?")
            result.add(
                Finding(
                    severity=Severity.ROJO,
                    title=f"{name}: cambia la version de almacenamiento (`{version}` → `{new_storage}`)",
                    detail=(
                        "Sin conversion webhook, los objetos ya guardados no se re-escriben solos "
                        "y quedan en la version vieja."
                    ),
                    remediation=(
                        "Forzar la reescritura de todos los objetos del kind "
                        "(`kubectl get <kind> -A -o yaml | kubectl replace -f -`) y recien despues "
                        "sacar la version vieja. " + BACKUP_HINT
                    ),
                )
            )

        _compare_version_schemas(
            result,
            name,
            version,
            (old_v.get("schema") or {}).get("openAPIV3Schema") or {},
            (new_v.get("schema") or {}).get("openAPIV3Schema") or {},
        )


def run(git: Git, config: dict[str, Any], base_ref: str, target_ref: str) -> CheckResult:
    result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)
    crd_dirs: list[str] = config["crd_dirs"]
    canonical_dir = crd_dirs[0]

    # La comparacion profunda corre SOLO sobre el directorio canonico. Los charts traen
    # copias del mismo CRD: compararlas tambien duplicaria cada hallazgo tantas veces como
    # copias haya. Que las copias esten al dia lo verifica el chequeo de divergencia de
    # mas abajo, que es la pregunta distinta.
    old_files = set(git.list_files(base_ref, canonical_dir))
    new_files = set(git.list_files(target_ref, canonical_dir))

    result.stats = {
        "canonical_dir": canonical_dir,
        "crd_files_base": len(old_files),
        "crd_files_target": len(new_files),
    }
    canonical_new = new_files

    expected = config.get("expected_crd_count")
    if expected is not None and len(canonical_new) != expected:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"El numero de CRDs cambio: {expected} → {len(canonical_new)}",
                detail=(
                    f"`{canonical_dir}` trae {len(canonical_new)} CRDs; el brief y "
                    "`idp-sync/upstream.json` esperan " + str(expected) + "."
                ),
                remediation=(
                    "Actualizar `expected_crd_count` en `idp-sync/upstream.json` junto con el bump, "
                    "y revisar en el brief cualquier referencia al numero."
                ),
            )
        )

    for file_path in sorted(old_files - new_files):
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"CRD eliminado: `{file_path}`",
                detail=(
                    "Borrar un CRD borra **todos** sus CRs en cascada. Si el bump se aplica con "
                    "`--prune` o alguien limpia a mano, se pierden objetos."
                ),
                remediation=(
                    "Confirmar en el CHANGELOG que el kind fue reemplazado, migrar los CRs y "
                    "NO borrar el CRD viejo hasta que no queden objetos. " + BACKUP_HINT
                ),
                refs=(file_path,),
            )
        )

    for file_path in sorted(new_files - old_files):
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"CRD nuevo: `{file_path}`",
                detail="Kind nuevo en el upstream.",
                remediation=APPLY_HINT,
                refs=(file_path,),
            )
        )

    changed = 0
    for file_path in sorted(old_files & new_files):
        old_raw = git.read_file(base_ref, file_path)
        new_raw = git.read_file(target_ref, file_path)
        if old_raw == new_raw:
            continue
        changed += 1
        old_crd = load_yaml(old_raw or "") or {}
        new_crd = load_yaml(new_raw or "") or {}
        _compare_crd(result, file_path, old_crd, new_crd)

    result.stats["crd_files_changed"] = changed

    # Divergencia entre la fuente (`config/crd/bases`) y lo que EMPAQUETA el chart.
    # Si no coinciden, `helm install` deja un CRD viejo aunque el repo tenga el nuevo.
    #
    # Se compara el YAML **parseado**, no los bytes: las copias del chart traen un `---`
    # inicial que no cambia nada. Comparar texto daria 36 hallazgos falsos en cada corrida,
    # que es la forma mas rapida de que el reporte deje de leerse.
    divergent: list[str] = []
    for chart_dir in crd_dirs[1:]:
        for file_path in git.list_files(target_ref, chart_dir):
            canonical = f"{canonical_dir}/{pathlib.PurePosixPath(file_path).name}"
            canonical_doc = load_yaml(git.read_file(target_ref, canonical) or "")
            chart_doc = load_yaml(git.read_file(target_ref, file_path) or "")
            if canonical_doc != chart_doc:
                divergent.append(file_path)

    if divergent:
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"{len(divergent)} CRD(s) del chart difieren del canonico `{canonical_dir}`",
                detail=(
                    "En el propio tag del upstream, lo que empaqueta el chart no es lo que dice el "
                    "repo. El chart instala su copia.\n\n"
                    + "\n".join(f"- `{p}`" for p in sorted(divergent)[:20])
                ),
                remediation=(f"Aplicar `{canonical_dir}/` a mano despues del helm y abrir issue upstream."),
                refs=tuple(sorted(divergent)),
            )
        )

    if changed and not any(f.severity is Severity.ROJO for f in result.findings):
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title=f"{changed} CRD(s) cambiaron sin achicar el schema",
                detail="Cambios aditivos o de metadata. No rompen CRs existentes.",
                remediation=APPLY_HINT,
            )
        )

    if not result.findings:
        result.summary = f"Sin cambios en los {len(canonical_new)} CRDs."
    else:
        result.summary = (
            f"{changed} de {len(canonical_new)} CRDs cambiaron. Recorda: `helm upgrade` NO toca `crds/`."
        )
    return result
