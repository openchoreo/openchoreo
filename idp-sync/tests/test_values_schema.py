# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Lo que distingue este check: una clave removida rompe fuerte o rompe en silencio."""

import pytest

from idp_sync.checks.values_schema import _compare, flatten_values, index_schema, run
from idp_sync.model import CheckResult, Severity


def schema(properties, *, closed=True, required=None):
    node = {
        "type": "object",
        "properties": properties,
        **({"required": required} if required else {}),
    }
    if closed:
        node["additionalProperties"] = False
    return node


@pytest.fixture
def result():
    return CheckResult(check_id="values-schema", order=2, title="t")


def test_flatten_values_devuelve_rutas_dotted():
    assert flatten_values({"gateway": {"enabled": False}, "security": {"enabled": True}}) == {
        "gateway",
        "gateway.enabled",
        "security",
        "security.enabled",
    }


def test_index_schema_anida_por_properties():
    idx = index_schema(schema({"gateway": schema({"enabled": {"type": "boolean"}})}))
    assert set(idx) == {"gateway", "gateway.enabled"}


def test_clave_removida_que_nosotros_seteamos_es_rojo(result):
    old = schema({"gateway": schema({"enabled": {"type": "boolean"}})})
    new = schema({"gateway": schema({})})
    _compare(result, "cp", old, new, our_values={"gateway", "gateway.enabled"})
    finding = next(f for f in result.findings if "clave removida" in f.title)
    assert finding.severity is Severity.ROJO
    assert "nosotros la seteamos" in finding.detail


def test_clave_removida_que_no_usamos_es_amarillo(result):
    old = schema({"thunder": schema({"baseUrl": {"type": "string"}})})
    new = schema({"thunder": schema({})})
    _compare(result, "cp", old, new, our_values={"gateway"})
    assert {f.severity for f in result.findings} == {Severity.AMARILLO}


def test_sin_values_propios_no_se_descarta_nada(result):
    old = schema({"gateway": schema({"enabled": {"type": "boolean"}})})
    new = schema({"gateway": schema({})})
    _compare(result, "cp", old, new, our_values=None)
    finding = next(f for f in result.findings if "clave removida" in f.title)
    assert finding.severity is Severity.AMARILLO
    assert "no se descarta que la usemos" in finding.detail


def test_el_detalle_distingue_falla_ruidosa_de_silenciosa(result_pair=None):
    ruidoso = CheckResult(check_id="c", order=2, title="t")
    _compare(
        ruidoso,
        "cp",
        schema({"gateway": schema({"enabled": {"type": "boolean"}})}),
        schema({"gateway": schema({})}),
        our_values=set(),
    )
    # `gateway.enabled` cuelga de `gateway`, que es cerrado -> helm falla.
    assert "falla**" in next(f for f in ruidoso.findings if "clave removida" in f.title).detail

    silencioso = CheckResult(check_id="c", order=2, title="t")
    _compare(
        silencioso,
        "cp",
        schema({"gateway": schema({"enabled": {"type": "boolean"}}, closed=False)}),
        schema({"gateway": schema({}, closed=False)}),
        our_values=set(),
    )
    assert "en silencio" in next(f for f in silencioso.findings if "clave removida" in f.title).detail


def test_clave_nueva_obligatoria_es_rojo(result):
    old = schema({"gateway": schema({})})
    new = schema({"gateway": schema({"className": {"type": "string"}}, required=["className"])})
    _compare(result, "cp", old, new, our_values=set())
    assert any("OBLIGATORIA" in f.title and f.severity is Severity.ROJO for f in result.findings)


def test_cambio_de_default_es_amarillo(result):
    old = schema({"security": schema({"enabled": {"type": "boolean", "default": False}})})
    new = schema({"security": schema({"enabled": {"type": "boolean", "default": True}})})
    _compare(result, "cp", old, new, our_values=set())
    finding = next(f for f in result.findings if "default" in f.title)
    assert finding.severity is Severity.AMARILLO


def test_schema_identico_no_genera_hallazgos(result):
    node = schema({"gateway": schema({"enabled": {"type": "boolean"}})})
    _compare(result, "cp", node, node, our_values=set())
    assert result.findings == []


def test_sin_platform_reporta_idp_platform_token_explicito():
    class FakeGit:
        def read_file(self, ref, path):  # noqa: ARG002
            return None

    res = run(
        FakeGit(),
        {"charts": [], "platform": {"values": {}}},
        "v1.2.2",
        "v1.3.0",
        platform_root=None,
        platform_missing_reason="Falta configurar el secret `IDP_PLATFORM_TOKEN`.",
    )
    finding = res.findings[0]
    assert finding.severity is Severity.AMARILLO
    assert "IDP_PLATFORM_TOKEN" in finding.title
    assert "IDP_PLATFORM_TOKEN" in finding.detail
