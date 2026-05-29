#!/usr/bin/env python3
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1] / "internal"

for path in ROOT.rglob("*.go"):
    text = path.read_text(encoding="utf-8")
    m = re.search(r"import \(", text)
    if not m:
        continue
    start = m.start()
    rest = text[start + len("import (") :]
    # find first line at column 0 that starts func/type/var/const after import
    m2 = re.search(r"\n(func |type |var |const )", rest)
    if not m2:
        continue
    block = rest[: m2.start()]
    if ")" in block:
        continue
    need_model = bool(re.search(r"\bmodel\.[A-Z]", text))
    need_biz = bool(re.search(r"\bbiz\.[A-Z]", text))
    lines = [ln for ln in block.splitlines() if ln.strip()]
    closing = []
    if need_model and not any("internal/data/model" in ln for ln in lines):
        closing.append('\t"kratos-demo/internal/data/model"')
    if need_biz and not any("internal/biz" in ln for ln in lines):
        closing.append('\t"kratos-demo/internal/biz"')
    closing.append(")")
    insert = "\n".join(closing) + "\n"
    new_text = text[: start + len("import (")] + block + insert + rest[m2.start() + 1 :]
    if new_text != text:
        path.write_text(new_text, encoding="utf-8", newline="\n")
        print("fixed", path)
