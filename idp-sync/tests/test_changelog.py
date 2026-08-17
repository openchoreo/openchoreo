# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""El caso que motiva todo el check: la deprecacion anunciada un release antes."""

from idp_sync.checks.changelog import analyze, parse_changelog, parse_version
from idp_sync.model import Severity

# Recorte fiel del CHANGELOG real: el anuncio de las Git secret APIs vive en la seccion
# de v1.1.0, y la seccion de v1.2.0 no lo menciona.
REAL_SHAPE = """
# Changelog

## v1.2.0

### Features

- **(Exec)** Interactive terminal access to running components. ([#3887](x))

### Bug Fixes

- **(Security)** Shell parameter injection fixed. ([#4193](x))

## v1.1.0

### Features

- **(Observability)** Wirelogs streamed via the API. ([#3571](x))

### Deprecation Notice

- Git secret management APIs are deprecated and removed in v1.2
- Deprecated cluster-prefixed mcp tools. Hidden in v1.2 and removed in v1.3 (https://x)

## v1.0.0-rc.2

### Breaking Changes

- **(CRD)** `REST` endpoint type removed from Workload. Use `HTTP` instead. ([#2785](x))
"""


def _titles(findings):
    return [f.title for f in findings]


def test_parse_version_ordena_prerelease_antes_del_final():
    assert parse_version("v1.2.0-rc.1") < parse_version("v1.2.0")
    assert parse_version("v1.2") == parse_version("v1.2.0")
    assert parse_version("no-es-version") is None


def test_parse_changelog_agrupa_bullets_por_seccion_y_heading():
    sections = parse_changelog(REAL_SHAPE)
    assert [s.raw_title for s in sections] == ["v1.2.0", "v1.1.0", "v1.0.0-rc.2"]
    deprecations = [b for h, b in sections[1].bullets if "Deprecation" in h]
    assert len(deprecations) == 2


def test_deprecacion_anunciada_antes_se_materializa_en_este_bump():
    # v1.1 -> v1.2: el anuncio esta en la seccion de v1.1, que YA corremos.
    findings = analyze(parse_changelog(REAL_SHAPE), parse_version("v1.1.0"), parse_version("v1.2.0"))
    materialized = [f for f in findings if "SE MATERIALIZA" in f.title]
    assert materialized, "la mina de v1.1 tiene que explotar al saltar a v1.2"
    assert all(f.severity is Severity.ROJO for f in materialized)
    assert any("Git secret management" in f.detail for f in materialized)


def test_la_misma_deprecacion_no_se_repite_una_vez_pasada():
    # v1.2.0 -> v1.2.2: la remocion "in v1.2" ya ocurrio, no vuelve a marcarse.
    findings = analyze(parse_changelog(REAL_SHAPE), parse_version("v1.2.0"), parse_version("v1.2.2"))
    assert not [f for f in findings if "SE MATERIALIZA" in f.title]


def test_deprecacion_futura_queda_en_amarillo_con_la_fecha_mas_temprana():
    findings = analyze(parse_changelog(REAL_SHAPE), parse_version("v1.0.0"), parse_version("v1.1.0"))
    futuras = [f for f in findings if "Deprecacion anunciada" in f.title]
    assert futuras
    assert all(f.severity is Severity.AMARILLO for f in futuras)
    # «Hidden in v1.2 and removed in v1.3»: el deadline que importa es el primero que muerde.
    mcp = next(f for f in futuras if "mcp tools" in f.detail)
    assert "v1.2.0" in mcp.remediation


def test_deprecacion_sin_version_de_retiro_lo_dice():
    text = """
## v1.1.0

### Deprecation Notice

- The `scopes.yaml` contract is deprecated.
"""
    findings = analyze(parse_changelog(text), parse_version("v1.0.0"), parse_version("v1.1.0"))
    finding = next(f for f in findings if "Deprecacion anunciada" in f.title)
    assert "no dice en que version se remueve" in finding.remediation


def test_breaking_changes_de_releases_intermedios_cuentan():
    # Saltar v1.0.0-rc.2 -> v1.2.0 tiene que arrastrar el breaking de rc.2... no:
    # rc.2 es la base. Lo que importa es que un salto largo tome TODAS las secciones.
    findings = analyze(parse_changelog(REAL_SHAPE), parse_version("v1.0.0-rc.1"), parse_version("v1.2.0"))
    assert any("REST` endpoint type removed" in f.detail for f in findings)


def test_sin_seccion_para_el_target_no_da_verde():
    findings = analyze(parse_changelog(REAL_SHAPE), parse_version("v1.2.0"), parse_version("v9.9.9"))
    assert any("no tiene seccion" in f.title for f in findings)


def test_remocion_fuera_de_breaking_changes_se_marca():
    text = """
## v2.0.0

### Bug Fixes

- **(API)** The `/legacy` prefix fallback removed. All clients must use current routes.
"""
    findings = analyze(parse_changelog(text), parse_version("v1.9.0"), parse_version("v2.0.0"))
    assert any("Remocion fuera de" in t for t in _titles(findings))
