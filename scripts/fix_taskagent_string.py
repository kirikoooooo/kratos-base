#!/usr/bin/env python3
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

REPLACEMENTS = [
    (re.compile(r"currentTaskAgent\(ctx\)\.String\(\)"), r"string(currentTaskAgent(ctx))"),
    (re.compile(r"biz\.TaskAgent(\w+)\.String\(\)"), r"string(biz.TaskAgent\1)"),
    (re.compile(r"(\w+)\.Agent\.String\(\)"), r"string(\1.Agent)"),
    (re.compile(r"\bagent\.IsEmpty\(\)"), r"strings.TrimSpace(string(agent)) == \"\""),
    (re.compile(r"(?<![.\w])agent\.String\(\)"), r"string(agent)"),
]

for path in ROOT.rglob("*.go"):
    if "vendor" in path.parts:
        continue
    text = path.read_text(encoding="utf-8")
    if "TaskAgent" not in text and "agent.String()" not in text and "agent.IsEmpty()" not in text:
        continue
    updated = text
    for pat, repl in REPLACEMENTS:
        updated = pat.sub(repl, updated)
    if updated != text:
        # ensure strings import in files that now use strings.TrimSpace for IsEmpty
        if "strings.TrimSpace(string(agent))" in updated and '"strings"' not in updated:
            if "import (" in updated:
                updated = updated.replace("import (", "import (\n\t\"strings\"", 1)
        path.write_text(updated, encoding="utf-8", newline="\n")
        print(path.relative_to(ROOT))
