# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Colisiones de parches, sobre el `PATCHES.md` real del espejo."""

import pathlib

from idp_sync.checks.patches import _covers, parse_patches_md, run
from idp_sync.model import Severity

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]

PATCHES_MD = """
# PATCHES — idp-openchoreo

## Parches vigentes

| # | Parche | Archivos | Motivo | Issue / PR upstream | Condicion de retiro |
|---|---|---|---|---|---|
| 0 | Fork | `.github/CODEOWNERS` | `.github/` gana sobre la raiz | — | Permanente |
| 1 | denylist | `internal/cluster-gateway/validator.go` | `/apis/v1/serviceaccounts` | #4237 | mergea |

## Parches planificados

| # | Parche | Archivos previstos | Motivo | Track |
|---|---|---|---|---|
| 2 | RBAC | `install/helm/openchoreo-data-plane/templates/agent/clusterrole.yaml` | comodin | T12 |
"""


class FakeGit:
    """Doble de `Git` con solo lo que consume el check."""

    def __init__(self, fork_files, upstream_changed, patch_commits=(), patches_md=PATCHES_MD):
        self._fork_files = list(fork_files)
        self._upstream_changed = list(upstream_changed)
        self._patch_commits = list(patch_commits)
        self._patches_md = patches_md

    def fork_diff_files(self, base, ours):
        return self._fork_files

    def log_subjects(self, base, ours):
        return [(sha, subject) for sha, subject, _ in self._patch_commits]

    def files_of_commit(self, sha):
        return next(files for s, _, files in self._patch_commits if s == sha)

    def read_file(self, ref, path):
        return self._patches_md if path == "PATCHES.md" else None

    def changed_files(self, base, target, paths=None):
        return self._upstream_changed


CONFIG = {"owned_paths": ["idp-sync/**", ".github/workflows/idp-upstream-sync.yml"]}


def test_parse_patches_md_solo_lee_los_vigentes():
    declared, rows = parse_patches_md(PATCHES_MD)
    assert declared == {".github/CODEOWNERS", "internal/cluster-gateway/validator.go"}
    assert len(rows) == 2
    # El de la tabla de "planificados" no cuenta: todavia no toca nada.
    assert not any("clusterrole.yaml" in d for d in declared)


def test_la_prosa_de_la_columna_motivo_no_declara_archivos():
    """`.github/` mencionado en «Motivo» cubriria todo `.github/` si leyeramos la fila entera."""
    declared, _ = parse_patches_md(PATCHES_MD)
    assert ".github/" not in declared
    assert "/apis/v1/serviceaccounts" not in declared
    assert not _covers(declared, ".github/README.md")


def test_covers_acepta_directorio_declarado():
    assert _covers({"internal/cluster-gateway/"}, "internal/cluster-gateway/validator_test.go")
    assert not _covers({"internal/cluster-gateway/"}, "internal/other/x.go")


def test_colision_es_rojo_y_avisa_del_revert_silencioso():
    git = FakeGit(
        fork_files=["internal/cluster-gateway/validator.go", ".github/CODEOWNERS"],
        upstream_changed=["internal/cluster-gateway/validator.go", "cmd/main.go"],
    )
    result = run(git, CONFIG, "v1.2.2", "v1.3.0")
    colision = next(f for f in result.findings if "Colision" in f.title)
    assert colision.severity is Severity.ROJO
    assert "revierte el parche" in colision.remediation
    assert result.stats["collisions"] == ["internal/cluster-gateway/validator.go"]


def test_sin_colision_no_hay_rojo():
    git = FakeGit(
        fork_files=["internal/cluster-gateway/validator.go"],
        upstream_changed=["cmd/main.go"],
    )
    result = run(git, CONFIG, "v1.2.2", "v1.3.0")
    assert result.severity is not Severity.ROJO


def test_archivo_nuestro_sin_fila_en_patches_md_es_amarillo():
    git = FakeGit(fork_files=["internal/nuevo/hack.go"], upstream_changed=[])
    result = run(git, CONFIG, "v1.2.2", "v1.3.0")
    finding = next(f for f in result.findings if "sin fila" in f.title)
    assert finding.severity is Severity.AMARILLO
    assert "internal/nuevo/hack.go" in finding.refs


def test_los_archivos_propios_del_agente_no_se_reportan_a_si_mismos():
    git = FakeGit(
        fork_files=[
            "idp-sync/idp_sync/report.py",
            "idp-sync/upstream.json",
            ".github/workflows/idp-upstream-sync.yml",
            ".github/CODEOWNERS",
        ],
        upstream_changed=[],
    )
    result = run(git, CONFIG, "v1.2.2", "v1.3.0")
    assert not [f for f in result.findings if "sin fila" in f.title]


def test_commits_patch_aportan_archivos_aunque_no_esten_en_el_diff():
    git = FakeGit(
        fork_files=[],
        upstream_changed=["agents/sre-agent/src/api/agent_routes.py"],
        patch_commits=[("abc123", "PATCH: rca-agent authn", ["agents/sre-agent/src/api/agent_routes.py"])],
    )
    result = run(git, CONFIG, "v1.2.2", "v1.3.0")
    assert any("Colision" in f.title for f in result.findings)
    assert result.stats["patch_commits"] == 1


def test_el_patches_md_real_del_repo_parsea():
    text = (REPO_ROOT / "PATCHES.md").read_text(encoding="utf-8")
    declared, rows = parse_patches_md(text)
    assert rows, "la tabla de parches vigentes tiene que parsear"
    assert ".github/CODEOWNERS" in declared
