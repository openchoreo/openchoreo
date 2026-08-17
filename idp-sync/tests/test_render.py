# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Semaforo, merge de reportes parciales y render markdown."""

import json
import pathlib

from idp_sync.checks.cr_validation import from_payload, run
from idp_sync.model import CheckResult, Finding, Report, Severity
from idp_sync.render import render_markdown, render_pr_title

SYNC_DIR = pathlib.Path(__file__).resolve().parents[1]


def result(check_id, order, severities, *, skipped=False):
    res = CheckResult(check_id=check_id, order=order, title=f"check {order}", summary="s")
    if skipped:
        res.skip("no se pudo mirar")
        return res
    for i, severity in enumerate(severities):
        res.add(Finding(severity=severity, title=f"h{i}", detail="d", remediation="r"))
    return res


def test_el_peor_gana():
    report = Report("v1.2.2", "v1.3.0", [result("a", 1, [Severity.VERDE, Severity.ROJO]), result("b", 2, [])])
    assert report.severity is Severity.ROJO


def test_un_check_salteado_nunca_es_verde():
    res = result("k3d", 6, [], skipped=True)
    assert res.severity is Severity.AMARILLO
    assert Report("a", "b", [res]).severity is Severity.AMARILLO


def test_merge_pisa_por_check_id_y_no_duplica():
    estatico = Report("v1.2.2", "v1.3.0", [result("crds", 1, [Severity.AMARILLO])])
    k3d = Report("v1.2.2", "v1.3.0", [result("cr-revalidation", 6, [Severity.ROJO])])
    merged = estatico.merge(k3d)
    assert [r.check_id for r in merged.ordered()] == ["crds", "cr-revalidation"]
    assert merged.severity is Severity.ROJO


def test_roundtrip_json():
    original = Report("v1.2.2", "v1.3.0", [result("crds", 1, [Severity.ROJO, Severity.AMARILLO])])
    restored = Report.from_dict(json.loads(json.dumps(original.to_dict())))
    assert restored.to_dict() == original.to_dict()


def test_markdown_pone_el_veredicto_arriba_y_lista_los_seis():
    report = Report(
        "v1.2.2",
        "v1.3.0",
        [result(f"c{i}", i, [Severity.AMARILLO]) for i in range(1, 7)],
    )
    md = render_markdown(report)
    lines = md.splitlines()
    assert lines[0].startswith("# 🟡")
    assert "Revision humana obligatoria" in lines[2]
    # Los checks aparecen en orden de riesgo, no de ejecucion.
    assert [
        int(line.split("|")[1])
        for line in lines
        if line.startswith("| ") and line.split("|")[1].strip().isdigit()
    ] == [1, 2, 3, 4, 5, 6]
    # El checklist de cierre no es decorativo: es el unico lugar donde vive el paso
    # manual de CRDs que `helm upgrade` no hace.
    assert "kubectl apply --server-side" in md


def test_markdown_marca_los_salteados_distinto_del_verde():
    md = render_markdown(Report("a", "b", [result("k3d", 6, [], skipped=True)]))
    assert "salteado" in md
    assert "**Salteado.** no se pudo mirar" in md


def test_titulo_del_pr_lleva_el_semaforo():
    report = Report("v1.2.2", "v1.3.0", [result("a", 1, [Severity.ROJO])])
    title = render_pr_title(report)
    assert title.startswith("[T08] 🔴")
    assert "v1.2.2 → v1.3.0" in title


def test_k3d_sin_crs_no_da_verde():
    res = from_payload({"cluster_ok": True, "crds_ok": True, "webhooks_ok": True, "crs": [], "note": ""})
    assert res.skipped
    assert res.severity is Severity.AMARILLO


def test_k3d_sin_platform_reporta_idp_platform_token_explicito(tmp_path):
    res = run(
        sync_dir=tmp_path,
        chart_dir=tmp_path,
        crd_dir=tmp_path,
        cr_dirs=[],
        values_file=None,
        out_file=tmp_path / "k3d.json",
        platform_missing_reason="Falta configurar el secret `IDP_PLATFORM_TOKEN`.",
    )
    assert res.skipped
    assert res.severity is Severity.AMARILLO
    assert "IDP_PLATFORM_TOKEN" in res.skip_reason


def test_k3d_con_cr_rechazado_es_rojo():
    res = from_payload(
        {
            "cluster_ok": True,
            "crds_ok": True,
            "webhooks_ok": True,
            "note": "",
            "crs": [
                {
                    "file": "abstractions/component-types/idp-service.yaml",
                    "ok": False,
                    "error": "denied by webhook",
                },
                {"file": "environments/stage.yaml", "ok": True, "error": ""},
            ],
        }
    )
    assert res.severity is Severity.ROJO
    assert res.stats == {"crs_total": 2, "crs_failed": 1}


def test_k3d_sin_webhooks_avisa_que_el_verde_vale_la_mitad():
    res = from_payload(
        {
            "cluster_ok": True,
            "crds_ok": True,
            "webhooks_ok": False,
            "note": "timeout",
            "crs": [{"file": "x.yaml", "ok": True, "error": ""}],
        }
    )
    assert res.severity is Severity.AMARILLO
    assert any("SIN webhooks" in f.title for f in res.findings)


def test_k3d_con_crds_que_no_aplican_es_rojo():
    res = from_payload({"cluster_ok": True, "crds_ok": False, "webhooks_ok": False, "crs": [], "note": ""})
    assert res.severity is Severity.ROJO


def test_el_config_del_repo_es_json_valido_y_completo():
    config = json.loads((SYNC_DIR / "upstream.json").read_text(encoding="utf-8"))
    assert config["pinned"]["tag"].startswith("v")
    assert len(config["charts"]) == 4
    assert config["crd_dirs"][0] == "config/crd/bases"
    assert "idp-sync/**" in config["owned_paths"]


def test_ensure_complete_rellena_los_checks_que_no_corrieron():
    from idp_sync.report import EXPECTED_CHECKS, ensure_complete

    solo_estatico = Report("v1.2.2", "v1.3.0", [result("crds", 1, [])])
    completo = ensure_complete(solo_estatico)
    assert {r.check_id for r in completo.results} == {c[0] for c in EXPECTED_CHECKS}
    # Un reporte de 5 filas se lee como "el sexto dio bien"; con la fila en amarillo, no.
    k3d = next(r for r in completo.results if r.check_id == "cr-revalidation")
    assert k3d.skipped and k3d.severity is Severity.AMARILLO
