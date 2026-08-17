# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Modelo del reporte: severidades, hallazgos y resultado por check.

El semaforo tiene tres estados y una sola regla de agregacion: **el peor gana**.
No hay promedios ni conteos ponderados — un solo ROJO tine todo el reporte de rojo,
porque un solo CRD con un campo removido alcanza para romper el upgrade.
"""

from __future__ import annotations

import dataclasses
import enum
from typing import Any


class Severity(enum.IntEnum):
    """Severidad del semaforo. El orden numerico ES la relacion "peor que"."""

    VERDE = 0
    AMARILLO = 1
    ROJO = 2

    @property
    def emoji(self) -> str:
        return {Severity.VERDE: "🟢", Severity.AMARILLO: "🟡", Severity.ROJO: "🔴"}[self]

    @property
    def label(self) -> str:
        return self.name.lower()

    @classmethod
    def worst(cls, severities: list[Severity]) -> Severity:
        return max(severities, default=cls.VERDE)

    @classmethod
    def parse(cls, value: str) -> Severity:
        return cls[value.strip().upper()]


@dataclasses.dataclass(frozen=True)
class Finding:
    """Un hallazgo concreto y accionable.

    `remediation` no es opcional por diseno: un hallazgo sin accion es ruido que
    entrena al revisor a ignorar el reporte.
    """

    severity: Severity
    title: str
    detail: str
    remediation: str
    refs: tuple[str, ...] = ()

    def to_dict(self) -> dict[str, Any]:
        return {
            "severity": self.severity.label,
            "title": self.title,
            "detail": self.detail,
            "remediation": self.remediation,
            "refs": list(self.refs),
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Finding:
        return cls(
            severity=Severity.parse(data["severity"]),
            title=data["title"],
            detail=data["detail"],
            remediation=data["remediation"],
            refs=tuple(data.get("refs", ())),
        )


@dataclasses.dataclass
class CheckResult:
    """Resultado de uno de los seis checks.

    `skipped` existe para que "no pude mirar" nunca se confunda con "esta todo bien".
    Un check salteado nunca es VERDE.
    """

    check_id: str
    order: int
    title: str
    findings: list[Finding] = dataclasses.field(default_factory=list)
    summary: str = ""
    skipped: bool = False
    skip_reason: str = ""
    stats: dict[str, Any] = dataclasses.field(default_factory=dict)

    @property
    def severity(self) -> Severity:
        if self.skipped:
            return Severity.AMARILLO
        return Severity.worst([f.severity for f in self.findings])

    def add(self, finding: Finding) -> Finding:
        self.findings.append(finding)
        return finding

    def skip(self, reason: str) -> None:
        self.skipped = True
        self.skip_reason = reason

    def sorted_findings(self) -> list[Finding]:
        # Estable: peor primero, y dentro de la misma severidad el orden de deteccion.
        return sorted(self.findings, key=lambda f: -int(f.severity))

    def to_dict(self) -> dict[str, Any]:
        return {
            "check_id": self.check_id,
            "order": self.order,
            "title": self.title,
            "severity": self.severity.label,
            "summary": self.summary,
            "skipped": self.skipped,
            "skip_reason": self.skip_reason,
            "stats": self.stats,
            "findings": [f.to_dict() for f in self.sorted_findings()],
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> CheckResult:
        result = cls(
            check_id=data["check_id"],
            order=data["order"],
            title=data["title"],
            summary=data.get("summary", ""),
            skipped=data.get("skipped", False),
            skip_reason=data.get("skip_reason", ""),
            stats=data.get("stats", {}),
        )
        result.findings = [Finding.from_dict(f) for f in data.get("findings", [])]
        return result


@dataclasses.dataclass
class Report:
    base_ref: str
    target_ref: str
    results: list[CheckResult] = dataclasses.field(default_factory=list)

    @property
    def severity(self) -> Severity:
        return Severity.worst([r.severity for r in self.results])

    def ordered(self) -> list[CheckResult]:
        return sorted(self.results, key=lambda r: r.order)

    def to_dict(self) -> dict[str, Any]:
        return {
            "base_ref": self.base_ref,
            "target_ref": self.target_ref,
            "severity": self.severity.label,
            "checks": [r.to_dict() for r in self.ordered()],
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Report:
        return cls(
            base_ref=data["base_ref"],
            target_ref=data["target_ref"],
            results=[CheckResult.from_dict(c) for c in data.get("checks", [])],
        )

    def merge(self, other: Report) -> Report:
        """Une reportes parciales (jobs distintos del workflow) por `check_id`."""
        by_id = {r.check_id: r for r in self.results}
        for result in other.results:
            by_id[result.check_id] = result
        return Report(
            base_ref=self.base_ref or other.base_ref,
            target_ref=self.target_ref or other.target_ref,
            results=list(by_id.values()),
        )
