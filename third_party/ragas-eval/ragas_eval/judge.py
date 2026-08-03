"""LLM-as-a-judge for qualitative evaluation of tool-calling behaviour.

Uses ragas-style prompting to assess whether the agent's tool calls were:
- Appropriate for the user query
- Correctly parameterized
- Efficiently sequenced
"""

import json
import os
from dataclasses import dataclass, field
from typing import Any, Dict, Optional

from .dataset import EvalCase, EvalDataset, EvalResult
from .metrics import AgentTrace, ToolCallRecord


# ── Judge prompts ───────────────────────────────────────────────────────────

TOOL_SELECTION_JUDGE_PROMPT = """你是一个 Agent 工具调用评估专家。请评估以下 Agent 的工具选择是否合理。

## 用户查询
{user_query}

## Agent 实际选择的工具
{selected_tools}

## 期望选择的工具
{expected_tools}

## 不应选择的工具
{forbidden_tools}

## 评估标准
1. 工具选择是否匹配用户意图？
2. 是否选择了不必要的工具？
3. 是否遗漏了关键工具？
4. 是否有更高效的工具选择方案？

请输出 JSON：
{{
    "score": 0.0-1.0,
    "reasoning": "评估理由（中文）",
    "issues": ["问题1", "问题2"],
    "suggestions": ["建议1", "建议2"]
}}
"""

PARAM_JUDGE_PROMPT = """你是一个 Agent 工具参数评估专家。请评估以下工具调用的参数是否正确。

## 用户查询
{user_query}

## 工具名称
{tool_name}

## 实际参数
```json
{actual_params}
```

## 期望参数（匹配模式: {params_mode}）
```json
{expected_params}
```

## 工具输出
{actual_output}

## 评估标准
1. 关键参数（如路径、命令、操作类型）是否正确？
2. 参数值是否合理（不越权、不超范围）？
3. 是否有遗漏的必需参数？

请输出 JSON：
{{
    "score": 0.0-1.0,
    "reasoning": "评估理由（中文）",
    "param_issues": [{{"param": "参数名", "issue": "问题描述"}}],
    "is_correct": true/false
}}
"""

MULTI_STEP_JUDGE_PROMPT = """你是一个 Agent 多步操作评估专家。请评估以下多步工具调用的顺序和完整性。

## 用户查询
{user_query}

## 实际工具调用序列
{actual_sequence}

## 期望工具调用序列
{expected_sequence}

## 评估标准
1. 工具调用顺序是否合理？
2. 是否有多余的步骤？
3. 是否缺少必要的步骤？
4. 步骤间的依赖关系是否正确？

请输出 JSON：
{{
    "score": 0.0-1.0,
    "reasoning": "评估理由（中文）",
    "sequence_issues": ["问题1"],
    "missing_steps": ["缺少的步骤"],
    "extra_steps": ["多余的步骤"]
}}
"""

ERROR_RECOVERY_JUDGE_PROMPT = """你是一个 Agent 错误恢复评估专家。请评估 Agent 在工具调用失败后的恢复能力。

## 用户查询
{user_query}

## 工具调用历史
{tool_call_history}

## 评估标准
1. Agent 是否正确识别了错误？
2. Agent 是否尝试了合理的替代方案？
3. Agent 是否避免了重复相同的失败调用？
4. Agent 最终是否向用户提供了有用的信息？

请输出 JSON：
{{
    "score": 0.0-1.0,
    "reasoning": "评估理由（中文）",
    "recovery_quality": "good" | "partial" | "poor",
    "issues": ["问题1"]
}}
"""


# ── LLM Judge client ────────────────────────────────────────────────────────

@dataclass
class LLMJudge:
    """Calls an LLM to produce qualitative evaluation scores.

    Reads provider configuration from environment variables:
    - OPENAI_API_KEY, OPENAI_BASE_URL
    - JUDGE_MODEL (default: gpt-4o-mini)
    """

    model: str = field(default_factory=lambda: os.getenv("JUDGE_MODEL", "gpt-4o-mini"))
    api_key: Optional[str] = field(default_factory=lambda: os.getenv("OPENAI_API_KEY"))
    base_url: Optional[str] = field(default_factory=lambda: os.getenv("OPENAI_BASE_URL"))

    def _call_llm(self, prompt: str) -> Dict[str, Any]:
        """Call the LLM and parse the JSON response."""
        try:
            from openai import OpenAI
        except ImportError:
            raise ImportError(
                "openai package is required for LLM judge. Install with: pip install openai"
            )

        if not self.api_key:
            raise ValueError("OPENAI_API_KEY environment variable is required for LLM judge")

        client_kwargs: Dict[str, Any] = {"api_key": self.api_key}
        if self.base_url:
            client_kwargs["base_url"] = self.base_url

        client = OpenAI(**client_kwargs)

        response = client.chat.completions.create(
            model=self.model,
            messages=[
                {
                    "role": "system",
                    "content": "你是一个专业的 Agent 工具调用评估专家。请始终输出有效的 JSON。",
                },
                {"role": "user", "content": prompt},
            ],
            temperature=0.0,
            response_format={"type": "json_object"},
        )

        content = response.choices[0].message.content
        if content is None:
            return {"score": 0.0, "reasoning": "LLM returned empty response", "error": True}

        try:
            return json.loads(content)
        except json.JSONDecodeError:
            # Try to extract JSON from the response
            content = content.strip()
            if content.startswith("```"):
                lines = content.split("\n")
                content = "\n".join(lines[1:-1])
            return json.loads(content)

    def judge_tool_selection(self, case: EvalCase, trace: AgentTrace) -> Dict[str, Any]:
        """Judge tool selection quality."""
        prompt = TOOL_SELECTION_JUDGE_PROMPT.format(
            user_query=case.user_query,
            selected_tools=json.dumps(
                [{"tool": tc.tool_name, "input": tc.input[:200]} for tc in trace.tool_calls],
                ensure_ascii=False,
                indent=2,
            ),
            expected_tools=json.dumps(case.expected_tools, ensure_ascii=False),
            forbidden_tools=json.dumps(case.forbidden_tools, ensure_ascii=False),
        )
        return self._call_llm(prompt)

    def judge_params(self, case: EvalCase, trace: AgentTrace) -> Dict[str, Any]:
        """Judge parameter extraction accuracy."""
        if case.expected_params is None:
            return {"score": 1.0, "reasoning": "No params to check", "skipped": True}

        matching_call = next(
            (tc for tc in trace.tool_calls if tc.tool_name == case.expected_params.tool_name),
            None,
        )

        if matching_call is None:
            return {
                "score": 0.0,
                "reasoning": f"Expected tool '{case.expected_params.tool_name}' was not called",
                "is_correct": False,
            }

        # Parse actual params
        try:
            if matching_call.input.strip().startswith("{"):
                actual_params = json.loads(matching_call.input)
            else:
                actual_params = {"input": matching_call.input.strip()}
        except json.JSONDecodeError:
            actual_params = {"input": matching_call.input.strip()}

        prompt = PARAM_JUDGE_PROMPT.format(
            user_query=case.user_query,
            tool_name=case.expected_params.tool_name,
            actual_params=json.dumps(actual_params, ensure_ascii=False, indent=2),
            expected_params=json.dumps(case.expected_params.params, ensure_ascii=False, indent=2),
            params_mode=case.expected_params.params_mode,
            actual_output=(matching_call.output or "")[:500],
        )
        return self._call_llm(prompt)

    def judge_multi_step(self, case: EvalCase, trace: AgentTrace) -> Dict[str, Any]:
        """Judge multi-step sequence quality."""
        if case.multi_step is None:
            return {"score": 1.0, "reasoning": "No multi-step sequence to check", "skipped": True}

        prompt = MULTI_STEP_JUDGE_PROMPT.format(
            user_query=case.user_query,
            actual_sequence=json.dumps(
                [
                    {
                        "step": i,
                        "tool": tc.tool_name,
                        "input": tc.input[:150],
                        "error": tc.error,
                    }
                    for i, tc in enumerate(trace.tool_calls)
                ],
                ensure_ascii=False,
                indent=2,
            ),
            expected_sequence=json.dumps(
                [
                    {"step": i, "tool": s.tool_name, "params": s.params}
                    for i, s in enumerate(case.multi_step.steps)
                ],
                ensure_ascii=False,
                indent=2,
            ),
        )
        return self._call_llm(prompt)

    def judge_error_recovery(self, case: EvalCase, trace: AgentTrace) -> Dict[str, Any]:
        """Judge error recovery quality."""
        has_errors = any(tc.error is not None for tc in trace.tool_calls)
        if not has_errors:
            return {"score": 1.0, "reasoning": "No errors to recover from", "skipped": True}

        prompt = ERROR_RECOVERY_JUDGE_PROMPT.format(
            user_query=case.user_query,
            tool_call_history=json.dumps(
                [
                    {
                        "step": i,
                        "tool": tc.tool_name,
                        "input": tc.input[:200],
                        "output": (tc.output or "")[:200],
                        "error": tc.error,
                    }
                    for i, tc in enumerate(trace.tool_calls)
                ],
                ensure_ascii=False,
                indent=2,
            ),
        )
        return self._call_llm(prompt)
