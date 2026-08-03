#!/usr/bin/env python3
"""Fix broken string literals in runtime.go by line number."""
from __future__ import annotations

import re
from pathlib import Path

PATH = Path(__file__).resolve().parents[1] / "internal/data/agent/runtime.go"

LINE_FIXES: dict[int, str] = {
    313: '\tsystemPrompt := strings.Join([]string{',
    314: '\t\t"你是一个通用代码代理运行时，负责直接理解任务并给出可执行结果。",',
    315: '\t\t"当任务涉及查看仓库、修改文件或执行本地验证时，优先调用可用函数，不要假设工具已经执行。",',
    316: '\t\t"可用工具覆盖读文件、edit_file 增量编辑、write_file 新建文件和执行受限验证命令；拿到工具结果后，再用中文给出真实总结。",',
    317: '\t\t"如果任务不需要工具，也可以直接回答，但不能编造执行结果。",',
    318: '\t}, "\\n")',
    320: '\treturn r.runToolCallingLoop(ctx, modelClient, toolset, systemPrompt, prompt, "default agent profile 已通过通用 runtime 完成任务")',
    435: '\tsystemText := strings.TrimSpace(systemPrompt) + "\\n修改已有文件必须先 read_file，再用 edit_file 原子操作：insert_line/insert_after_line/prepend/append 插入，delete_line/delete_lines/delete_string 删除，replace_line/replace_lines/search_replace 替换；行号从 1 开始；write_file 仅新建文件。\\n分析目录结构时对目录调用 read_file（如 read_file internal/biz）会返回条目列表，再逐个 read_file 具体 .go 文件；不要依赖 exec_command 做目录搜索。\\n若 read_file 因路径不存在失败，先尝试 read_file 父目录或修正相对路径；exec_command 全任务最多 3 次且失败后会禁用，优先 read_file。"',
    438: '\t\t\tsystemText += "\\n\\n## Agent Memory（提示词调优）\\n" + block',
    616: '\t\t"请根据这个错误修正参数、路径或工具选择后重试。",',
    751: '\t\t"用户任务：",',
    898: '\t\treturn agentName + " 未返回结果。"',
    902: '\tparts = append(parts, agentName+" 已完成子任务。")',
    904: '\t\tparts = append(parts, "总结："+summary)',
    915: '\t\tprompt = "请验证 router -> " + agent.String() + " 的委派链路"',
}


def main() -> None:
    text = PATH.read_text(encoding="utf-8-sig")
    lines = text.splitlines()
    for lineno, replacement in LINE_FIXES.items():
        lines[lineno - 1] = replacement
    text = "\n".join(lines) + "\n"

    broken = [
        i + 1
        for i, line in enumerate(text.splitlines())
        if re.search(r'"[^"]*\?,\s*$', line) or re.search(r'[^"]+"[^"]*\?\s*$', line)
    ]
    if broken:
        raise SystemExit(f"Still broken lines: {broken}")

    PATH.write_text(text, encoding="utf-8", newline="\n")
    print("Fixed", PATH)


if __name__ == "__main__":
    main()
