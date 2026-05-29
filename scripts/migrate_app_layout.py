#!/usr/bin/env python3
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPLACEMENTS = [
    ("kratos-demo/internal/conf", "kratos-demo/app/kratos-demo/internal/conf"),
    ("kratos-demo/internal/service", "kratos-demo/app/kratos-demo/internal/service"),
    ("kratos-demo/internal/server", "kratos-demo/app/kratos-demo/internal/server"),
    ("go run ./cmd/kratos-demo", "go run ./app/kratos-demo/cmd"),
    ("cd cmd/kratos-demo && wire", "cd app/kratos-demo/cmd && wire"),
]

SKIP = {".git", ".agents", "scripts"}

for path in ROOT.rglob("*"):
    if path.suffix not in {".go", ".md", "Makefile"} and path.name != "Makefile":
        continue
    if any(part in SKIP for part in path.parts):
        continue
    text = path.read_text(encoding="utf-8")
    updated = text
    for old, new in REPLACEMENTS:
        updated = updated.replace(old, new)
    if updated != text:
        path.write_text(updated, encoding="utf-8", newline="\n")
        print(path.relative_to(ROOT))
