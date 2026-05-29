#!/usr/bin/env python3
"""Migrate internal/data/model references to domain packages."""

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

TASKING_TYPES = {
    "Task", "TaskStatus", "TaskRepo", "TaskDispatcher",
    "ErrPromptRequired", "ErrTaskNotFound", "ErrAgentNotSupported", "ErrTaskDispatcherUnavailable",
    "TaskStatusPending", "TaskStatusRunning", "TaskStatusDone", "TaskStatusFailed",
}

SESSION_TYPES = {
    "Session", "SessionStore", "SessionFileChange", "SessionFileOp",
}

AGENT_TYPES = {
    "AgentMemoryStore", "AgentMemory", "AgentMemoryConfig",
    "ConversationRole", "ConversationRoleHuman", "ConversationRoleAI", "ConversationRoleTool",
    "ConversationToolCall", "ConversationTurn", "SessionConversation",
    "CommandPolicy", "ToolHint", "SkillHint", "PromptAdjustment",
    "UserAgentMemory", "SessionAgentMemory", "SessionErrorRecord",
    "ContextUsageSnapshot", "ContextPrepareMeta", "ContextCompressConfig",
    "ContextCompressStats", "ContextCompressResult",
    "DefaultContextCompressThreshold", "DefaultKeepRecentTurns", "DefaultToolOutputMaxChars",
}

TRACE_TYPES = {
    "DelegationTraceStore", "DelegationTraceSubscriber",
    "DelegationEvent", "PlanStep", "DelegationSession",
}

PACKAGE_IMPORTS = {
    "tasking": ('datatasking "kratos-demo/internal/data/tasking"', TASKING_TYPES),
    "session": ('datasession "kratos-demo/internal/data/session"', SESSION_TYPES),
    "agent": ('dataagent "kratos-demo/internal/data/agent"', AGENT_TYPES),
    "trace": ('datatrace "kratos-demo/internal/data/trace"', TRACE_TYPES),
}

PACKAGE_PREFIX = {
    "tasking": "datatasking.",
    "session": "datasession.",
    "agent": "dataagent.",
    "trace": "datatrace.",
}

LOCAL_PACKAGE = {
    "internal/data/tasking": "tasking",
    "internal/data/session": "session",
    "internal/data/agent": "agent",
    "internal/data/trace": "trace",
}


def type_package(name: str) -> str | None:
    if name in TASKING_TYPES:
        return "tasking"
    if name in SESSION_TYPES:
        return "session"
    if name in AGENT_TYPES:
        return "agent"
    if name in TRACE_TYPES:
        return "trace"
    return None


def replace_model_refs(content: str, local: str | None) -> tuple[str, set[str]]:
    needed: set[str] = set()

    def replacer(match: re.Match) -> str:
        name = match.group(1)
        pkg = type_package(name)
        if pkg is None:
            return match.group(0)
        if local == pkg:
            return name
        needed.add(pkg)
        return PACKAGE_PREFIX[pkg] + name

    content = re.sub(r"\bmodel\.([A-Za-z_][A-Za-z0-9_]*)", replacer, content)
    return content, needed


def strip_model_import(content: str) -> str:
    content = re.sub(r'\t"kratos-demo/internal/data/model"\n', "", content)
    content = re.sub(r'\n\t"kratos-demo/internal/data/model"', "", content)
    content = re.sub(
        r'import \(\n\t"kratos-demo/internal/data/model"\n\)',
        "",
        content,
    )
    return content


def add_imports(content: str, needed: set[str], local: str | None) -> str:
    needed = {pkg for pkg in needed if pkg != local}
    if not needed:
        return content

    import_lines = []
    for pkg in sorted(needed):
        import_lines.append("\t" + PACKAGE_IMPORTS[pkg][0])

    if re.search(r"^import \(", content, re.M):
        def inject(match: re.Match) -> str:
            block = match.group(0)
            for line in import_lines:
                if line.strip() not in block:
                    block = block.rstrip(")\n") + "\n" + line + "\n)\n"
            return block

        content = re.sub(r"import \([\s\S]*?\)\n", inject, content, count=1)
    else:
        content = re.sub(
            r"^(import .+\n)",
            r"\1import (\n" + "\n".join(import_lines) + "\n)\n",
            content,
            count=1,
            flags=re.M,
        )
    return content


def process_file(path: Path) -> bool:
    rel = path.relative_to(ROOT).as_posix()
    if rel.startswith("internal/data/model/"):
        return False
    if "kratos-demo/internal/data/model" not in path.read_text(encoding="utf-8"):
        return False

    local = None
    for prefix, pkg in LOCAL_PACKAGE.items():
        if rel.startswith(prefix):
            local = pkg
            break

    content = path.read_text(encoding="utf-8")
    content = strip_model_import(content)
    content, needed = replace_model_refs(content, local)
    content = add_imports(content, needed, local)
    path.write_text(content, encoding="utf-8")
    print(f"updated {rel}")
    return True


def main() -> None:
    count = 0
    for path in ROOT.rglob("*.go"):
        if process_file(path):
            count += 1
    print(f"done: {count} files")


if __name__ == "__main__":
    main()
