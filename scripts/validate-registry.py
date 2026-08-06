#!/usr/bin/env python3
"""Validate the bundled CLIProxyAPI plugin registry files."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from urllib.parse import urlparse

ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
VERSION_RE = re.compile(r"^[0-9][0-9A-Za-z.+-]*$")
REQUIRED = ("id", "name", "description", "author", "repository")
FILES = ("registry.json", "registry.zh-CN.json", "registry.en.json")


def validate_repository(value: str) -> None:
    parsed = urlparse(value)
    parts = [part for part in parsed.path.split("/") if part]
    if parsed.scheme != "https" or parsed.netloc != "github.com" or len(parts) != 2:
        raise ValueError("repository must be https://github.com/{owner}/{repo}")
    if parsed.query or parsed.fragment:
        raise ValueError("repository must not contain query or fragment")


def validate_file(path: Path) -> list[str]:
    warnings: list[str] = []
    data = json.loads(path.read_text(encoding="utf-8"))
    if data.get("schema_version") not in (1, 2):
        raise ValueError(f"{path.name}: schema_version must be 1 or 2")
    plugins = data.get("plugins")
    if not isinstance(plugins, list) or not plugins:
        raise ValueError(f"{path.name}: plugins must be a non-empty array")
    seen: set[str] = set()
    for index, plugin in enumerate(plugins):
        if not isinstance(plugin, dict):
            raise ValueError(f"{path.name}: plugins[{index}] must be an object")
        for field in REQUIRED:
            if not str(plugin.get(field, "")).strip():
                raise ValueError(f"{path.name}: plugins[{index}] missing {field}")
        plugin_id = str(plugin["id"]).strip()
        if not ID_RE.fullmatch(plugin_id):
            raise ValueError(f"{path.name}: invalid plugin id {plugin_id!r}")
        if plugin_id in seen:
            raise ValueError(f"{path.name}: duplicate plugin id {plugin_id!r}")
        seen.add(plugin_id)
        version = str(plugin.get("version", "")).strip()
        if version and (version.startswith("v") or not VERSION_RE.fullmatch(version)):
            raise ValueError(f"{path.name}: invalid version {version!r}")
        repository = str(plugin["repository"]).strip()
        validate_repository(repository)
        if "/OWNER/" in repository:
            warnings.append(f"{path.name}: replace OWNER with the real GitHub owner before publishing")
    return warnings


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    warnings: list[str] = []
    for name in FILES:
        path = root / name
        if not path.exists():
            raise FileNotFoundError(path)
        warnings.extend(validate_file(path))
        print(f"OK  {path.name}")
    for warning in sorted(set(warnings)):
        print(f"WARN {warning}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001
        print(f"ERROR {exc}", file=sys.stderr)
        raise SystemExit(1)
