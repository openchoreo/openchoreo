# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""El check de CRDs: lo que achica el schema es rojo, lo aditivo es amarillo."""

import copy

import pytest

from idp_sync.checks.crds import _compare_crd, _index_schema
from idp_sync.model import CheckResult, Severity


def crd(properties, *, required=None, versions=("v1alpha1",), storage="v1alpha1", served=True):
    return {
        "metadata": {"name": "components.openchoreo.dev"},
        "spec": {
            "versions": [
                {
                    "name": name,
                    "served": served,
                    "storage": name == storage,
                    "schema": {
                        "openAPIV3Schema": {
                            "type": "object",
                            "properties": {
                                "spec": {
                                    "type": "object",
                                    "properties": properties,
                                    **({"required": required} if required else {}),
                                }
                            },
                        }
                    },
                }
                for name in versions
            ]
        },
    }


@pytest.fixture
def result():
    return CheckResult(check_id="crds", order=1, title="t")


def severities(result):
    return {f.severity for f in result.findings}


def test_index_schema_recorre_properties_items_y_additional():
    index = _index_schema(
        {
            "type": "object",
            "properties": {
                "spec": {
                    "type": "object",
                    "properties": {
                        "containers": {
                            "type": "array",
                            "items": {
                                "type": "object",
                                "properties": {"image": {"type": "string"}},
                            },
                        },
                        "labels": {"type": "object", "additionalProperties": {"type": "string"}},
                    },
                }
            },
        }
    )
    assert "spec.containers[].image" in index
    assert "spec.labels{}" in index


def test_propiedad_removida_es_rojo(result):
    old = crd({"image": {"type": "string"}, "replicas": {"type": "integer"}})
    new = crd({"image": {"type": "string"}})
    _compare_crd(result, "f.yaml", old, new)
    rojos = [f for f in result.findings if f.severity is Severity.ROJO]
    assert any("propiedad removida" in f.title for f in rojos)
    assert "silencio" in rojos[0].detail


def test_propiedad_agregada_no_es_rojo(result):
    old = crd({"image": {"type": "string"}})
    new = crd({"image": {"type": "string"}, "command": {"type": "array"}})
    _compare_crd(result, "f.yaml", old, new)
    assert Severity.ROJO not in severities(result)


def test_campo_que_pasa_a_obligatorio_es_rojo(result):
    old = crd({"image": {"type": "string"}, "tag": {"type": "string"}})
    new = crd({"image": {"type": "string"}, "tag": {"type": "string"}}, required=["tag"])
    _compare_crd(result, "f.yaml", old, new)
    assert any("obligatorio" in f.title and f.severity is Severity.ROJO for f in result.findings)


def test_enum_achicado_es_rojo(result):
    old = crd({"kind": {"type": "string", "enum": ["deployment", "proxy", "cronjob"]}})
    new = crd({"kind": {"type": "string", "enum": ["deployment", "cronjob"]}})
    _compare_crd(result, "f.yaml", old, new)
    finding = next(f for f in result.findings if "enum" in f.title)
    assert finding.severity is Severity.ROJO
    assert "proxy" in finding.detail


def test_cambio_de_tipo_es_rojo_y_cambio_de_default_es_amarillo(result):
    old = crd({"replicas": {"type": "integer", "default": 1}})
    new = crd({"replicas": {"type": "string", "default": 2}})
    _compare_crd(result, "f.yaml", old, new)
    by_key = {f.title.split("cambio ")[-1]: f for f in result.findings}
    assert by_key["`type`"].severity is Severity.ROJO
    assert by_key["`default`"].severity is Severity.AMARILLO


def test_version_removida_es_rojo_y_avisa_que_no_hay_conversion(result):
    old = crd({"image": {"type": "string"}}, versions=("v1alpha1", "v1beta1"), storage="v1beta1")
    new = crd({"image": {"type": "string"}}, versions=("v1beta1",), storage="v1beta1")
    _compare_crd(result, "f.yaml", old, new)
    finding = next(f for f in result.findings if "removida" in f.title)
    assert finding.severity is Severity.ROJO
    assert "conversion webhooks" in finding.detail


def test_cambio_de_version_de_almacenamiento_es_rojo(result):
    old = crd({"image": {"type": "string"}}, versions=("v1alpha1", "v1beta1"), storage="v1alpha1")
    new = crd({"image": {"type": "string"}}, versions=("v1alpha1", "v1beta1"), storage="v1beta1")
    _compare_crd(result, "f.yaml", old, new)
    assert any("almacenamiento" in f.title and f.severity is Severity.ROJO for f in result.findings)


def test_dejar_de_preservar_campos_desconocidos_es_rojo(result):
    old = crd({"overrides": {"type": "object", "x-kubernetes-preserve-unknown-fields": True}})
    new = crd({"overrides": {"type": "object"}})
    _compare_crd(result, "f.yaml", old, new)
    assert any("campos desconocidos" in f.title and f.severity is Severity.ROJO for f in result.findings)


def test_crd_identico_no_genera_hallazgos(result):
    base = crd({"image": {"type": "string"}})
    _compare_crd(result, "f.yaml", base, copy.deepcopy(base))
    assert result.findings == []
