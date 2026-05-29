#!/usr/bin/env python3
"""Fix Go import blocks broken by migration script."""
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "internal"


def fix_imports(content: str, need_model: bool) -> str:
    pattern = re.compile(r"import \(\n(\n)(func |type |var )", re.MULTILINE)
    if not pattern.search(content):
        # also handle missing close after last import line
        pattern2 = re.compile(
            r"(import \(\n(?:[^\n]*\n)*?)(\n)(func |type |var )", re.MULTILINE
        )
        m = pattern2.search(content)
        if m:
            block = m.group(1)
            if ")" not in block:
                insert = '\t"kratos-demo/internal/data/model"\n)\n\n' if need_model else ")\n\n"
                content = content[: m.start(2)] + insert + content[m.start(3) :]
        return content
    insert = '\t"kratos-demo/internal/data/model"\n)\n\n' if need_model else ")\n\n"
    return pattern.sub(rf"import (\n{insert}\1", content)


def needs_model(content: str) -> bool:
    return bool(re.search(r"\bmodel\.[A-Z]", content))


def main() -> None:
    changed = []
    for path in ROOT.rglob("*.go"):
        original = path.read_text(encoding="utf-8")
        if "import (" not in original:
            continue
        # detect unclosed import: count parens in first import block
        idx = original.find("import (")
        end = original.find("\nfunc ", idx)
        if end == -1:
            end = original.find("\ntype ", idx)
        if end == -1:
            end = original.find("\nvar ", idx)
        if end == -1:
            continue
        block = original[idx:end]
        if block.count("(") > block.count(")"):
            updated = fix_imports(original, needs_model(original))
            if updated != original:
                path.write_text(updated, encoding="utf-8", newline="\n")
                changed.append(path)
    print(f"Fixed {len(changed)} files")
    for p in changed:
        print(" ", p.relative_to(ROOT.parent))


if __name__ == "__main__":
    main()
