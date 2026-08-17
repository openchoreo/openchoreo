# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""CLI del agente de sync.

    python -m idp_sync latest-tag                      # resuelve el ultimo release del upstream
    python -m idp_sync analyze  --target v1.3.0 -o static.json
    python -m idp_sync validate --target v1.3.0 -o k3d.json
    python -m idp_sync render   -i static.json -i k3d.json -m report.md -j findings.json

`analyze` y `validate` son subcomandos separados a proposito: corren en jobs distintos
del workflow para que un k3d que se cuelga no se lleve puesto el reporte estatico.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys

from .model import Report, Severity
from .render import render_markdown, render_pr_title
from .report import ensure_complete, run_k3d, run_static
from .util import Git, load_config

SYNC_DIR = pathlib.Path(__file__).resolve().parent.parent


def _config(args: argparse.Namespace) -> dict:
    return load_config(pathlib.Path(args.sync_dir))


def _platform_root(args: argparse.Namespace) -> pathlib.Path | None:
    if not args.platform_root:
        return None
    root = pathlib.Path(args.platform_root).resolve()
    return root if root.is_dir() else None


def _write(path: str | None, content: str) -> None:
    if path:
        pathlib.Path(path).write_text(content, encoding="utf-8")


def cmd_latest_tag(args: argparse.Namespace) -> int:
    """Ultimo tag de release del upstream, ignorando rc/m/alpha."""
    config = _config(args)
    git = Git(pathlib.Path(args.repo_root))
    pattern = re.compile(config["upstream"]["release_tag_pattern"])
    tags = [t.strip() for t in git.run("tag", "--list").splitlines() if pattern.match(t.strip())]
    if not tags:
        print("", end="")
        return 1

    def key(tag: str) -> tuple[int, ...]:
        return tuple(int(part) for part in tag.lstrip("v").split("."))

    latest = max(tags, key=key)
    pinned = config["pinned"]["tag"]
    print(json.dumps({"latest": latest, "pinned": pinned, "is_new": key(latest) > key(pinned)}))
    return 0


def cmd_analyze(args: argparse.Namespace) -> int:
    config = _config(args)
    git = Git(pathlib.Path(args.repo_root))
    base = args.base or config["pinned"]["tag"]
    report = run_static(
        git=git,
        config=config,
        base_ref=base,
        target_ref=args.target,
        platform_root=_platform_root(args),
        platform_missing_reason=args.platform_missing_reason,
        our_ref=args.our_ref,
        skip=tuple(args.skip or ()),
    )
    _write(args.out, json.dumps(report.to_dict(), indent=2, ensure_ascii=False))
    print(f"{report.severity.emoji} {report.severity.label}", file=sys.stderr)
    return 0


def cmd_validate(args: argparse.Namespace) -> int:
    config = _config(args)
    git = Git(pathlib.Path(args.repo_root))
    base = args.base or config["pinned"]["tag"]
    workdir = pathlib.Path(args.workdir).resolve()
    workdir.mkdir(parents=True, exist_ok=True)
    report = run_k3d(
        git=git,
        config=config,
        sync_dir=pathlib.Path(args.sync_dir).resolve(),
        base_ref=base,
        target_ref=args.target,
        platform_root=_platform_root(args),
        workdir=workdir,
        platform_missing_reason=args.platform_missing_reason,
    )
    _write(args.out, json.dumps(report.to_dict(), indent=2, ensure_ascii=False))
    print(f"{report.severity.emoji} {report.severity.label}", file=sys.stderr)
    return 0


def cmd_render(args: argparse.Namespace) -> int:
    merged: Report | None = None
    for path in args.inputs:
        candidate = pathlib.Path(path)
        if not candidate.is_file():
            continue
        report = Report.from_dict(json.loads(candidate.read_text(encoding="utf-8")))
        merged = report if merged is None else merged.merge(report)

    if merged is None:
        print("No hubo ningun reporte parcial que renderizar.", file=sys.stderr)
        return 2

    merged = ensure_complete(merged)
    _write(args.markdown, render_markdown(merged))
    _write(args.json, json.dumps(merged.to_dict(), indent=2, ensure_ascii=False))
    if args.title:
        _write(args.title, render_pr_title(merged))

    print(merged.severity.label)
    if args.exit_code:
        return {Severity.VERDE: 0, Severity.AMARILLO: 0, Severity.ROJO: 1}[merged.severity]
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="idp_sync", description=__doc__)
    parser.add_argument("--repo-root", default=".", help="Raiz del espejo idp-openchoreo.")
    parser.add_argument("--sync-dir", default=str(SYNC_DIR), help="Directorio idp-sync/.")
    sub = parser.add_subparsers(dest="command", required=True)

    latest = sub.add_parser("latest-tag", help="Ultimo tag de release del upstream.")
    latest.set_defaults(func=cmd_latest_tag)

    analyze = sub.add_parser("analyze", help="Checks 0-5 (sin cluster).")
    analyze.add_argument("--target", required=True)
    analyze.add_argument("--base", default=None, help="Por default, el tag fijado en upstream.json.")
    analyze.add_argument("--our-ref", default="HEAD", help="Nuestra rama con los parches.")
    analyze.add_argument("--platform-root", default=None, help="Checkout de idp-platform.")
    analyze.add_argument(
        "--platform-missing-reason",
        default="",
        help="Mensaje explicito para reportar por que no esta disponible idp-platform.",
    )
    analyze.add_argument("--skip", action="append", help="check_id a saltear. Repetible.")
    analyze.add_argument("-o", "--out", required=True)
    analyze.set_defaults(func=cmd_analyze)

    validate = sub.add_parser("validate", help="Check 6 (k3d efimero).")
    validate.add_argument("--target", required=True)
    validate.add_argument("--base", default=None)
    validate.add_argument("--platform-root", default=None)
    validate.add_argument(
        "--platform-missing-reason",
        default="",
        help="Mensaje explicito para reportar por que no esta disponible idp-platform.",
    )
    validate.add_argument("--workdir", default=".idp-sync-work")
    validate.add_argument("-o", "--out", required=True)
    validate.set_defaults(func=cmd_validate)

    render = sub.add_parser("render", help="Une reportes parciales y emite el markdown.")
    render.add_argument("-i", "--inputs", action="append", required=True)
    render.add_argument("-m", "--markdown", default=None)
    render.add_argument("-j", "--json", default=None)
    render.add_argument("-t", "--title", default=None)
    render.add_argument("--exit-code", action="store_true", help="Salir != 0 si el semaforo es rojo.")
    render.set_defaults(func=cmd_render)

    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
