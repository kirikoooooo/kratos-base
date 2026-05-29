#!/usr/bin/env python3
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPLACEMENTS = [
    ("kratos-demo/app/kratos-demo/internal/conf", "kratos-demo/internal/conf"),
    ("app/kratos-demo/internal/conf/conf.proto", "internal/conf/conf.proto"),
]

for path in ROOT.rglob("*"):
    if path.suffix not in {".go", ".md"} and path.name != "Makefile":
        continue
    text = path.read_text(encoding="utf-8")
    updated = text
    for old, new in REPLACEMENTS:
        updated = updated.replace(old, new)
    if updated != text:
        path.write_text(updated, encoding="utf-8", newline="\n")
        print(path.relative_to(ROOT))
