# Copyright 2026 fondomp-production
# SPDX-License-Identifier: Apache-2.0
"""Utilidades compartidas: git, carga de config y lectura de YAML/JSON en un ref."""

from __future__ import annotations

import json
import pathlib
import subprocess
from typing import Any

import yaml

try:  # El loader en C es ~5x mas rapido; los CRDs son grandes.
    from yaml import CSafeLoader as _SafeLoader  # type: ignore[attr-defined]
except ImportError:  # pragma: no cover - depende del build de PyYAML
    from yaml import SafeLoader as _SafeLoader  # type: ignore[assignment]

CONFIG_FILENAME = "upstream.json"
MAX_DIFF_LINES = 120


class GitError(RuntimeError):
    pass


class Git:
    """Wrapper fino sobre `git` acotado al repo del espejo."""

    def __init__(self, repo_root: pathlib.Path):
        self.repo_root = pathlib.Path(repo_root).resolve()

    def run(self, *args: str, check: bool = True) -> str:
        proc = subprocess.run(
            ["git", *args],
            cwd=self.repo_root,
            capture_output=True,
            text=True,
        )
        if check and proc.returncode != 0:
            raise GitError(f"git {' '.join(args)} -> {proc.returncode}: {proc.stderr.strip()}")
        return proc.stdout

    def rev_parse(self, ref: str) -> str:
        # `^{commit}` desreferencia el tag anotado. Sin esto se compara el sha del
        # objeto tag, que no es el commit — el error que ya tiene PATCHES.md.
        return self.run("rev-parse", f"{ref}^{{commit}}").strip()

    def ref_exists(self, ref: str) -> bool:
        try:
            self.rev_parse(ref)
            return True
        except GitError:
            return False

    def list_files(self, ref: str, path: str) -> list[str]:
        out = self.run("ls-tree", "-r", "--name-only", ref, "--", path, check=False)
        return [line for line in out.splitlines() if line.strip()]

    def read_file(self, ref: str, path: str) -> str | None:
        proc = subprocess.run(
            ["git", "show", f"{ref}:{path}"],
            cwd=self.repo_root,
            capture_output=True,
            text=True,
        )
        if proc.returncode != 0:
            return None
        return proc.stdout

    def changed_files(self, base: str, target: str, paths: list[str] | None = None) -> list[str]:
        args = ["diff", "--name-only", f"{base}..{target}"]
        if paths:
            args += ["--", *paths]
        return [line for line in self.run(*args).splitlines() if line.strip()]

    def fork_diff_files(self, upstream_ref: str, our_ref: str) -> list[str]:
        # Tres puntos: lo que cambiamos NOSOTROS desde el punto de divergencia,
        # sin arrastrar lo que avanzo el upstream.
        out = self.run("diff", "--name-only", f"{upstream_ref}...{our_ref}")
        return [line for line in out.splitlines() if line.strip()]

    def log_subjects(self, base: str, target: str) -> list[tuple[str, str]]:
        out = self.run("log", "--format=%H%x1f%s", f"{base}..{target}")
        entries = []
        for line in out.splitlines():
            if "\x1f" in line:
                sha, subject = line.split("\x1f", 1)
                entries.append((sha, subject))
        return entries

    def files_of_commit(self, sha: str) -> list[str]:
        out = self.run("show", "--name-only", "--format=", sha)
        return [line for line in out.splitlines() if line.strip()]

    def add_worktree(self, ref: str, dest: pathlib.Path) -> None:
        self.run("worktree", "add", "--detach", str(dest), ref)

    def remove_worktree(self, dest: pathlib.Path) -> None:
        self.run("worktree", "remove", "--force", str(dest), check=False)


def load_config(sync_dir: pathlib.Path) -> dict[str, Any]:
    with (pathlib.Path(sync_dir) / CONFIG_FILENAME).open(encoding="utf-8") as handle:
        return json.load(handle)


def load_yaml(text: str) -> Any:
    return yaml.load(text, Loader=_SafeLoader)


def load_yaml_documents(text: str) -> list[Any]:
    return [doc for doc in yaml.load_all(text, Loader=_SafeLoader) if doc is not None]


def truncate(text: str, max_lines: int = MAX_DIFF_LINES) -> str:
    lines = text.splitlines()
    if len(lines) <= max_lines:
        return text
    omitted = len(lines) - max_lines
    return "\n".join(lines[:max_lines] + [f"… (+{omitted} lineas omitidas)"])


def match_glob(path: str, patterns: list[str]) -> bool:
    """`fnmatch` no trata `/` de forma especial, asi que `idp-sync/**` matchea anidados."""
    import fnmatch

    return any(fnmatch.fnmatch(path, pattern) for pattern in patterns)
