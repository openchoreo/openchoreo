# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Render markdown del reporte: el semaforo.

Reglas de presentacion, en orden de importancia:

1. **El veredicto va primero**, en la primera linea. Quien abre el PR tiene que poder
   decidir sin scrollear.
2. **El orden de los checks es el orden de riesgo**, no el de ejecucion.
3. Un check salteado se ve distinto de un check verde. Nunca se colapsan.
"""

from __future__ import annotations

from .model import Report, Severity

VERDICT = {
    Severity.ROJO: (
        "**NO MERGEAR TAL CUAL.** Hay al menos un cambio que rompe CRs, values o el render. "
        "Cada hallazgo rojo trae la accion que lo desbloquea."
    ),
    Severity.AMARILLO: (
        "**Revision humana obligatoria.** Nada rompe de entrada, pero hay cambios que requieren "
        "decision o un paso manual (empezando por aplicar los CRDs, que `helm upgrade` no toca)."
    ),
    Severity.VERDE: (
        "**Sin hallazgos.** Igual vale releer el diff del upstream: el agente mira las seis "
        "superficies del brief, no todo el repo."
    ),
}

CHECKLIST = """
## Antes de aplicar el bump

- [ ] Backup de los 37 kinds del control plane: `kubectl get <kind> -A -o yaml`. Los CRs son el
      estado autoritativo y **los CRDs no se revierten solos**.
- [ ] Backup de los Secrets del namespace del control plane.
- [ ] `kubectl apply --server-side --force-conflicts -f config/crd/bases/` **antes** del
      `helm upgrade`. Helm no actualiza `crds/` en upgrade.
- [ ] Rebasear los parches vigentes y **verificar que el fix siga aplicado** (los dos CRITICAL
      son de seguridad; resolver el conflicto "tomando el de upstream" los revierte sin ruido).
- [ ] Actualizar `idp-sync/upstream.json` **y** `PATCHES.md` con el tag y el commit nuevos.
""".strip()


def render_markdown(report: Report) -> str:
    severity = report.severity
    lines: list[str] = [
        f"# {severity.emoji} Sync upstream · `{report.base_ref}` → `{report.target_ref}`",
        "",
        VERDICT[severity],
        "",
        "| # | Check | Semaforo | Resumen |",
        "|---|---|---|---|",
    ]

    for result in report.ordered():
        summary = result.skip_reason if result.skipped else (result.summary or "—")
        # Escapado a mano: un `|` sin escapar parte la fila de la tabla en markdown.
        summary = summary.replace("|", "\\|")
        state = f"{result.severity.emoji} salteado" if result.skipped else result.severity.emoji
        lines.append(f"| {result.order} | {result.title} | {state} | {summary} |")

    lines += ["", "---", ""]

    for result in report.ordered():
        lines.append(f"## {result.severity.emoji} {result.order}. {result.title}")
        lines.append("")
        if result.skipped:
            lines += [f"> **Salteado.** {result.skip_reason}", ""]
        elif not result.findings:
            lines += [result.summary or "Sin hallazgos.", ""]
            continue
        else:
            lines += [result.summary, ""]

        for finding in result.sorted_findings():
            lines.append(f"<details><summary>{finding.severity.emoji} <b>{finding.title}</b></summary>")
            lines += ["", finding.detail, ""]
            lines.append(f"**Que hacer:** {finding.remediation}")
            if finding.refs:
                lines.append("")
                lines.append("Archivos: " + ", ".join(f"`{ref}`" for ref in finding.refs))
            lines += ["", "</details>", ""]

    lines += ["---", "", CHECKLIST, ""]
    lines.append(
        "<sub>Generado por `idp-sync`. Fuente de verdad del proceso: "
        "`docs/08-infrastructure/idp-openchoreo/README.md` del repo `architecture`.</sub>"
    )
    return "\n".join(lines)


def render_pr_title(report: Report) -> str:
    return (
        f"[T08] {report.severity.emoji} sync upstream {report.base_ref} → {report.target_ref} "
        f"({report.severity.label})"
    )
