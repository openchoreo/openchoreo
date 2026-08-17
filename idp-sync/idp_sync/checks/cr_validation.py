# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Check 6 — revalidacion de TODOS los CRs de `idp-platform` en un k3d efimero.

Es el unico check que ejerce los **webhooks** del tag nuevo. La mitad de lo que nos puede
romper no esta en el schema del CRD: vive en las `preRenderValidations` CEL de
ComponentTypes y Traits, y en los webhooks del controller-manager. Un validador offline
tipo `kubeconform` no las ve y devuelve verde.

El trabajo pesado lo hace `idp-sync/scripts/k3d-validate-crs.sh`; aca solo se traduce su
JSON a hallazgos. Si el cluster no levanta, el check queda **amarillo**: "no pude mirar"
no es "esta todo bien".
"""

from __future__ import annotations

import json
import pathlib
import subprocess
from typing import Any

from ..model import CheckResult, Finding, Severity

CHECK_ID = "cr-revalidation"
ORDER = 6
TITLE = "Revalidacion de los CRs en k3d"

SCRIPT = "scripts/k3d-validate-crs.sh"
DEFAULT_TIMEOUT = 45 * 60


def from_payload(payload: dict[str, Any]) -> CheckResult:
    """Traduce el JSON del script a hallazgos. Separado para poder testearlo sin k3d."""
    result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)

    if not payload.get("cluster_ok"):
        result.skip("No se pudo crear el k3d efimero. " + (payload.get("note") or ""))
        return result
    if not payload.get("crds_ok"):
        result.add(
            Finding(
                severity=Severity.ROJO,
                title="Los CRDs del tag nuevo no se pudieron aplicar en un cluster limpio",
                detail=(
                    "`kubectl apply --server-side` fallo o algun CRD no llego a `Established`. "
                    "Es exactamente el comando que hay que correr a mano en el upgrade real, "
                    "porque `helm upgrade` no toca `crds/`."
                ),
                remediation="Sin esto no hay upgrade posible. Bloquear el bump y leer el error del job.",
            )
        )
        return result

    crs = payload.get("crs", [])
    if not crs:
        result.skip(
            "No habia CRs para validar. "
            + (payload.get("note") or "")
            + " Mientras `idp-platform` este vacio este check no puede dar senal."
        )
        return result

    if not payload.get("webhooks_ok"):
        result.add(
            Finding(
                severity=Severity.AMARILLO,
                title="El control plane no llego a Ready: los CRs se validaron SIN webhooks",
                detail=(
                    (payload.get("note") or "") + "\n\nLos CRs pasaron solo la validacion de schema. Las "
                    "`preRenderValidations` CEL de ComponentTypes y Traits no se ejercieron."
                ),
                remediation="Revisar el log del job; sin webhooks el verde de este check vale la mitad.",
            )
        )

    failed = [cr for cr in crs if not cr.get("ok")]
    for cr in failed:
        result.add(
            Finding(
                severity=Severity.ROJO,
                title=f"CR rechazado: `{cr.get('file')}`",
                detail=f"```\n{cr.get('error', '')}\n```",
                remediation=(
                    "Corregir el CR en `idp-platform` en el MISMO PR del bump. Si el rechazo viene "
                    "de un webhook, el error no aparece en ningun diff de schema."
                ),
                refs=(str(cr.get("file")),),
            )
        )

    result.stats = {"crs_total": len(crs), "crs_failed": len(failed)}
    result.summary = (
        f"{len(crs) - len(failed)}/{len(crs)} CRs validos contra los CRDs y webhooks de la version nueva."
    )
    return result


def run(
    sync_dir: pathlib.Path,
    chart_dir: pathlib.Path,
    crd_dir: pathlib.Path,
    cr_dirs: list[pathlib.Path],
    values_file: pathlib.Path | None,
    out_file: pathlib.Path,
    platform_missing_reason: str = "",
    timeout: int = DEFAULT_TIMEOUT,
) -> CheckResult:
    if not cr_dirs:
        result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)
        if platform_missing_reason:
            reason = (
                "No hay checkout de `idp-platform`. "
                + platform_missing_reason
                + " Levantar un k3d para validar cero archivos seria un verde vacio."
            )
        else:
            reason = (
                "No hay checkout de `idp-platform` o no tiene CRs todavia. "
                "Levantar un k3d para validar cero archivos seria un verde vacio."
            )
        result.skip(reason + " Este check queda amarillo, nunca verde.")
        return result

    args = [
        "bash",
        str(sync_dir / SCRIPT),
        "--chart-dir",
        str(chart_dir),
        "--crd-dir",
        str(crd_dir),
        "--out",
        str(out_file),
    ]
    for cr_dir in cr_dirs:
        args += ["--cr-dir", str(cr_dir)]
    if values_file is not None:
        args += ["--values", str(values_file)]

    try:
        subprocess.run(args, timeout=timeout, check=False)
    except subprocess.TimeoutExpired:
        result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)
        result.skip(f"El script de k3d supero el timeout de {timeout}s.")
        return result

    if not out_file.is_file():
        result = CheckResult(check_id=CHECK_ID, order=ORDER, title=TITLE)
        result.skip("El script de k3d no dejo resultado.")
        return result

    return from_payload(json.loads(out_file.read_text(encoding="utf-8")))
