#!/usr/bin/env python3
"""Migrate biz domain types to internal/data/model."""
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

MODEL_TYPES = [
    "Task",
    "TaskStatus",
    "TaskRepo",
    "TaskDispatcher",
    "TaskUsecase",
    "NewTaskUsecase",
    "ErrPromptRequired",
    "ErrTaskNotFound",
    "ErrAgentNotSupported",
    "ErrTaskDispatcherUnavailable",
    "AgentMemoryStore",
    "AgentMemory",
    "AgentMemoryConfig",
    "ConversationRole",
    "ConversationRoleHuman",
    "ConversationRoleAI",
    "ConversationRoleTool",
    "ConversationToolCall",
    "ConversationTurn",
    "SessionConversation",
    "CommandPolicy",
    "ToolHint",
    "SkillHint",
    "PromptAdjustment",
    "UserAgentMemory",
    "SessionAgentMemory",
    "SessionErrorRecord",
    "ContextUsageSnapshot",
    "ContextPrepareMeta",
    "DefaultContextCompressThreshold",
    "DefaultKeepRecentTurns",
    "DefaultToolOutputMaxChars",
    "ContextCompressConfig",
    "ContextCompressStats",
    "ContextCompressResult",
    "DelegationTraceStore",
    "DelegationTraceSubscriber",
    "DelegationEvent",
    "PlanStep",
    "DelegationSession",
    "SessionChangeStore",
    "SessionFileChange",
    "SessionFileOp",
]

BIZ_KEEP = [
    "TaskAgent",
    "TaskAgentDefault",
    "TaskAgentGeneric",
    "TaskAgentRouter",
    "TaskAgentCoder",
    "TaskAgentReviewer",
    "AgentRuntime",
    "DelegationVerifier",
    "NewTaskUsecase",  # removed from biz
]

SKIP_DIRS = {".git", ".agents", "scripts", "third_party", "api", "doc", "docs", "configs"}


def should_process(path: Path) -> bool:
    if path.suffix != ".go":
        return False
    parts = set(path.parts)
    if parts & SKIP_DIRS:
        return False
    if path.parent.name == "biz" and path.name != "agent.go":
        return False
    if path.parent.name == "model":
        return False
    return True


def add_model_import(content: str) -> str:
    if "kratos-demo/internal/data/model" in content:
        return content
    if not any(f"biz.{t}" in content for t in MODEL_TYPES):
        return content
    lines = content.splitlines()
    insert_at = 0
    in_import = False
    for i, line in enumerate(lines):
        if line.startswith("import ("):
            in_import = True
            insert_at = i + 1
            continue
        if in_import:
            if line.strip() == ")":
                lines.insert(insert_at, '\t"kratos-demo/internal/data/model"')
                return "\n".join(lines) + ("\n" if content.endswith("\n") else "")
        if line.startswith("import "):
            # single import line
            module = line.split('"')[1]
            lines[i] = f"import (\n\t\"{module}\"\n\t\"kratos-demo/internal/data/model\"\n)"
            return "\n".join(lines) + ("\n" if content.endswith("\n") else "")
    return content


def replace_types(content: str) -> str:
    for name in MODEL_TYPES:
        content = re.sub(rf"\bbiz\.{name}\b", f"model.{name}", content)
    content = content.replace("biz.NewTaskUsecase", "tasking.NewTaskUsecase")
    content = content.replace("normalizeAgent(", "tasking.NormalizeAgent(")
    content = content.replace("normalizedUserID()", "NormalizedUserID()")
    return content


def maybe_add_tasking_import(content: str, path: Path) -> str:
    if "tasking.NewTaskUsecase" not in content and "tasking.NormalizeAgent" not in content:
        return content
    if path.parent.name == "tasking":
        return content
    if "kratos-demo/internal/data/tasking" in content:
        return content
    lines = content.splitlines()
    for i, line in enumerate(lines):
        if line.startswith("import ("):
            lines.insert(i + 1, '\tdatatasking "kratos-demo/internal/data/tasking"')
            content = "\n".join(lines) + "\n"
            return content.replace("tasking.NewTaskUsecase", "datatasking.NewTaskUsecase").replace(
                "tasking.NormalizeAgent", "datatasking.NormalizeAgent"
            )
    return content


def cleanup_biz_import(content: str) -> str:
    if "kratos-demo/internal/biz" not in content:
        return content
    uses_biz = bool(re.search(r"\bbiz\.[A-Za-z]", content))
    if uses_biz:
        return content
    lines = []
    skip = False
    for line in content.splitlines():
        if '"kratos-demo/internal/biz"' in line:
            skip = True
            continue
        if skip and line.strip() == ")":
            skip = False
            continue
        if '"kratos-demo/internal/biz"' in line and line.startswith("import "):
            continue
        lines.append(line)
    return "\n".join(lines) + ("\n" if content.endswith("\n") else "")


def main() -> None:
    changed = []
    for path in ROOT.rglob("*.go"):
        if not should_process(path):
            continue
        original = path.read_text(encoding="utf-8")
        updated = original
        updated = replace_types(updated)
        updated = add_model_import(updated)
        updated = maybe_add_tasking_import(updated, path)
        updated = cleanup_biz_import(updated)
        if updated != original:
            path.write_text(updated, encoding="utf-8", newline="\n")
            changed.append(path.relative_to(ROOT))
    print(f"Updated {len(changed)} files")
    for p in sorted(changed):
        print(" ", p)


if __name__ == "__main__":
    main()
