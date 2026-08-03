"""Evaluation metrics for kratos-base tool-calling capabilities.

Core metrics are powered by ragas:
- ToolCallAccuracy — tool name sequencing + argument accuracy (ragas)
- ToolCallF1 — set-based tool call precision/recall/F1 (ragas)

Supplementary metrics for dimensions ragas does not cover:
- multi_step_compliance — strict ordering via LCS
- error_recovery_score — retry/fallback quality

All ragas metrics use the official MultiTurnSample API:
  sample = MultiTurnSample(user_input=[AIMessage(...)], reference_tool_calls=[ToolCall(...)])
  score = await ToolCallAccuracy().ascore(user_input, reference_tool_calls)
"""

import asyncio
from typing import Any, Dict, List, Optional, Tuple

from pydantic import BaseModel, Field

from .dataset import EvalCase


# ── Raw tool call record (from Go trace or direct observation) ──────────────

class ToolCallRecord(BaseModel):
    """A single observed tool call from the agent."""

    tool_name: str
    input: str
    output: Optional[str] = None
    error: Optional[str] = None
    exit_code: Optional[int] = None
    duration_ms: Optional[float] = None
    step_index: int = 0


class AgentTrace(BaseModel):
    """Complete agent interaction trace."""

    task_id: str
    user_query: str
    tool_calls: List[ToolCallRecord] = Field(default_factory=list)
    final_response: Optional[str] = None
    metadata: Dict[str, Any] = Field(default_factory=dict)


# ── ragas ToolCallAccuracy ───────────────────────────────────────────────────

def _tool_call_accuracy_score(case: EvalCase, trace: AgentTrace) -> float:
    """Evaluate tool call accuracy using ragas ToolCallAccuracy.

    Strategy by category:
    - tool_selection, error_recovery: name-only (name matches = accuracy 1.0)
    - param_extraction: name-only for tool naming + targeted _param_accuracy for args
    - multi_step: full name+args with strict_order

    Returns 0.0–1.0.
    """
    from ragas.metrics.collections import ToolCallAccuracy

    from .adapters import build_ragas_sample

    sample = build_ragas_sample(case, trace)
    if sample.reference_tool_calls is None or len(sample.reference_tool_calls) == 0:
        return 1.0

    # For param_extraction: use name-only accuracy (tool selection validated by ragas)
    # and compute parameter accuracy separately against expected_params
    if case.category == "param_extraction":
        # Name-only: all ref + act tools get empty args
        from ragas.messages import ToolCall as RagaToolCall, AIMessage as RagaAIMessage

        name_ref = [RagaToolCall(name=r.name, args={}) for r in sample.reference_tool_calls]
        name_act: List[RagaToolCall] = []
        for msg in sample.user_input:
            if msg.tool_calls:
                name_act.extend([RagaToolCall(name=t.name, args={}) for t in msg.tool_calls])

        tca = ToolCallAccuracy(strict_order=False)
        try:
            result = asyncio.run(tca.ascore(
                [RagaAIMessage(content="", tool_calls=name_act)],
                name_ref,
            ))
            name_score = float(result.value) if hasattr(result, 'value') else float(result)
        except Exception:
            name_score = 0.0

        # Also compute param accuracy for the specific expected_params tool
        param_score = _computed_param_accuracy(case, trace)
        # Combined: name accuracy (50%) + param accuracy (50%)
        return name_score * 0.5 + param_score * 0.5

    # For multi_step: validate tool name sequence with ragas, params with custom modes
    if case.multi_step is not None:
        # Name-only accuracy (ragas validates the tool name sequence)
        from ragas.messages import ToolCall as RagaToolCall, AIMessage as RagaAIMessage

        name_ref = [RagaToolCall(name=r.name, args={}) for r in sample.reference_tool_calls]
        name_act: List[RagaToolCall] = []
        for msg in sample.user_input:
            if msg.tool_calls:
                name_act.extend([RagaToolCall(name=t.name, args={}) for t in msg.tool_calls])

        strict = case.multi_step.strict_order
        tca_seq = ToolCallAccuracy(strict_order=strict)
        try:
            result = asyncio.run(tca_seq.ascore(
                [RagaAIMessage(content="", tool_calls=name_act)],
                name_ref,
            ))
            name_score = float(result.value) if hasattr(result, 'value') else float(result)
        except Exception:
            name_score = 0.0

        # Custom param matching with per-step params_mode
        param_score = _multi_step_param_accuracy(case, trace)
        return name_score * 0.5 + param_score * 0.5

    # For tool_selection / error_recovery: name-only (already handled by adapter)
    strict = False
    tca = ToolCallAccuracy(strict_order=strict)
    try:
        result = asyncio.run(tca.ascore(sample.user_input, sample.reference_tool_calls))
        return float(result.value) if hasattr(result, 'value') else float(result)
    except Exception:
        return 0.0


def _computed_param_accuracy(case: EvalCase, trace: AgentTrace) -> float:
    """Check that the expected_params tool was called with matching parameters.

    Supports three match modes via ToolExpectation.params_mode:
    - exact: string equality
    - contains: substring match
    - subset: all expected keys present with matching values
    """
    import json as _json

    if case.expected_params is None:
        return 1.0

    ep = case.expected_params

    # Find matching tool calls
    matching = [tc for tc in trace.tool_calls if tc.tool_name == ep.tool_name]
    if not matching:
        return 0.0

    best = 0.0
    for call in matching:
        # Parse observed params
        observed: Dict[str, Any] = {}
        raw = call.input.strip()
        if raw.startswith("{"):
            try:
                observed = _json.loads(raw)
            except _json.JSONDecodeError:
                observed = {"input": raw}
        else:
            observed = {"input": raw}

        if not ep.params:
            best = max(best, 1.0)
            continue

        matched = 0
        for key, exp_val in ep.params.items():
            obs_val = observed.get(key)
            if obs_val is None:
                continue

            mode = ep.params_mode
            if mode == "exact":
                if str(obs_val) == str(exp_val):
                    matched += 1
            elif mode == "contains":
                if str(exp_val).lower() in str(obs_val).lower():
                    matched += 1
            elif mode == "subset":
                if isinstance(exp_val, dict) and isinstance(obs_val, dict):
                    if all(str(obs_val.get(k, "")) == str(v) for k, v in exp_val.items()):
                        matched += 1
                elif str(obs_val) == str(exp_val):
                    matched += 1

        score = matched / len(ep.params)
        best = max(best, score)

    return best


# ── ragas ToolCallF1 ─────────────────────────────────────────────────────────

def _tool_call_f1_score(case: EvalCase, trace: AgentTrace) -> float:
    """Evaluate tool call F1 using ragas ToolCallF1.

    Uses name-only comparison (empty args on both sides) because F1 measures
    tool selection quality, not parameter accuracy. Parameter accuracy is
    handled separately by ToolCallAccuracy.

    Returns 0.0–1.0.
    """
    from ragas.metrics.collections import ToolCallF1

    from .adapters import build_ragas_sample

    sample = build_ragas_sample(case, trace)
    if sample.reference_tool_calls is None or len(sample.reference_tool_calls) == 0:
        return 1.0

    # Strip args for name-only F1 — this is about tool selection, not parameters
    from ragas.messages import AIMessage as RagaAIMessage, ToolCall as RagaToolCall

    name_only_ref = [RagaToolCall(name=r.name, args={}) for r in sample.reference_tool_calls]
    name_only_act: List[RagaToolCall] = []
    for msg in sample.user_input:
        if msg.tool_calls:
            name_only_act.extend([RagaToolCall(name=t.name, args={}) for t in msg.tool_calls])

    f1_metric = ToolCallF1()
    try:
        result = asyncio.run(f1_metric.ascore(
            [RagaAIMessage(content="", tool_calls=name_only_act)],
            name_only_ref,
        ))
        return float(result.value) if hasattr(result, 'value') else float(result)
    except Exception:
        return 0.0


# ── Supplementary: tool-level success rate ───────────────────────────────────

def tool_call_success_rate(trace: AgentTrace) -> float:
    """Proportion of tool calls that succeeded (no error field set)."""
    if not trace.tool_calls:
        return 1.0
    successful = sum(1 for tc in trace.tool_calls if tc.error is None)
    return successful / len(trace.tool_calls)


def _multi_step_param_accuracy(case: EvalCase, trace: AgentTrace) -> float:
    """Validate multi-step parameters using the declared params_mode per step.

    Unlike ragas ExactMatch, this supports 'contains' and 'subset' modes.
    """
    import json as _json

    if case.multi_step is None:
        return 1.0

    steps = case.multi_step.steps
    step_scores: List[float] = []

    for i, step in enumerate(steps):
        # Find matching tool call by position (strict order) or by name
        if case.multi_step.strict_order and i < len(trace.tool_calls):
            call = trace.tool_calls[i]
        else:
            matches = [tc for tc in trace.tool_calls if tc.tool_name == step.tool_name]
            if not matches:
                continue
            call = matches[0]

        if call.tool_name != step.tool_name:
            continue

        # Parse observed params
        observed: Dict[str, Any] = {}
        raw = call.input.strip()
        if raw.startswith("{"):
            try:
                observed = _json.loads(raw)
            except _json.JSONDecodeError:
                observed = {"input": raw}
        else:
            observed = {"input": raw}

        if not step.params:
            step_scores.append(1.0)
            continue

        matched = 0
        for key, exp_val in step.params.items():
            obs_val = observed.get(key)
            if obs_val is None:
                continue

            mode = step.params_mode
            if mode == "exact":
                if str(obs_val) == str(exp_val):
                    matched += 1
            elif mode == "contains":
                if str(exp_val).lower() in str(obs_val).lower():
                    matched += 1
            elif mode == "subset":
                if isinstance(exp_val, dict) and isinstance(obs_val, dict):
                    if all(str(obs_val.get(k, "")) == str(v) for k, v in exp_val.items()):
                        matched += 1
                elif str(obs_val) == str(exp_val):
                    matched += 1

        step_scores.append(matched / len(step.params))

    if not step_scores:
        return 1.0
    return sum(step_scores) / len(step_scores)


# ── Supplementary: multi-step compliance (LCS) ───────────────────────────────

def multi_step_compliance(case: EvalCase, trace: AgentTrace) -> float:
    """Evaluate multi-step sequence order using longest common subsequence.

    ragas ToolCallAccuracy with strict_order=True handles name sequencing,
    but doesn't provide a standalone LCS-based score. This metric gives
    fine-grained sequence alignment for reporting.
    """
    if case.multi_step is None:
        return 1.0

    expected_names = [s.tool_name for s in case.multi_step.steps]
    observed_names = [tc.tool_name for tc in trace.tool_calls]

    if not expected_names:
        return 1.0

    if case.multi_step.strict_order:
        match_count = _lcs_length(expected_names, observed_names)
        score = match_count / len(expected_names)
    else:
        observed_set = set(observed_names)
        expected_set = set(expected_names)
        score = len(expected_set & observed_set) / len(expected_set)

    if not case.multi_step.allow_extra_steps and len(observed_names) > len(expected_names):
        score *= 0.8

    return score


def _lcs_length(a: List[str], b: List[str]) -> int:
    """Longest common subsequence length (dynamic programming)."""
    m, n = len(a), len(b)
    dp = [[0] * (n + 1) for _ in range(m + 1)]
    for i in range(m):
        for j in range(n):
            if a[i] == b[j]:
                dp[i + 1][j + 1] = dp[i][j] + 1
            else:
                dp[i + 1][j + 1] = max(dp[i + 1][j], dp[i][j + 1])
    return dp[m][n]


# ── Supplementary: error recovery score ──────────────────────────────────────

def error_recovery_score(case: EvalCase, trace: AgentTrace) -> float:
    """Evaluate the agent's ability to recover from tool call failures.

    ragas does not have an error-recovery metric. This provides:
    - Uniqueness ratio: did the agent avoid repeating the same error?
    - Eventual success: did the agent recover with a successful tool call?
    """
    if not case.expected_tools:
        return 1.0

    expected_failures = 0
    if case.expected_params and not case.expected_params.should_succeed:
        expected_failures += 1
    if case.multi_step:
        expected_failures += sum(1 for s in case.multi_step.steps if not s.should_succeed)

    if expected_failures == 0:
        return 1.0

    failure_sigs: List[Tuple[str, str]] = []
    for tc in trace.tool_calls:
        if tc.error is not None:
            sig = (tc.tool_name, tc.error[:100] if tc.error else "")
            failure_sigs.append(sig)

    total = len(failure_sigs)
    if total == 0:
        return 0.7  # Agent avoided expected failures entirely

    uniqueness_ratio = len(set(failure_sigs)) / total

    eventual_success = any(
        tc.error is None and tc.tool_name in case.expected_tools
        for tc in trace.tool_calls
    )

    base = uniqueness_ratio * 0.6
    if eventual_success:
        base += 0.4

    return min(base, 1.0)


# ── Evaluate one case ────────────────────────────────────────────────────────

def evaluate_case(case: EvalCase, trace: AgentTrace) -> Dict[str, Any]:
    """Evaluate all applicable metrics for a single case.

    Core metrics (ragas):
    - tool_call_accuracy : ragas ToolCallAccuracy
    - tool_call_f1       : ragas ToolCallF1

    Supplementary metrics:
    - multi_step_compliance : LCS-based sequence score
    - error_recovery_score  : retry/fallback quality
    - tool_call_success_rate: raw tool success proportion
    """
    results: Dict[str, float] = {}

    # ragas: tool call naming + argument accuracy
    results["tool_call_accuracy"] = _tool_call_accuracy_score(case, trace)

    # ragas: set-based F1 for tool calls
    results["tool_call_f1"] = _tool_call_f1_score(case, trace)

    # Supplementary
    if case.multi_step is not None:
        results["multi_step_compliance"] = multi_step_compliance(case, trace)

    results["tool_call_success_rate"] = tool_call_success_rate(trace)
    results["error_recovery_score"] = error_recovery_score(case, trace)

    # Pass/fail: all metrics >= 0.7 (except raw success rate)
    threshold = 0.7
    key_metrics = [
        v for k, v in results.items()
        if k not in ("tool_call_success_rate",)
    ]
    passed = all(v >= threshold for v in key_metrics) if key_metrics else True

    return {
        "case_id": case.id,
        "category": case.category,
        "passed": passed,
        "metrics": results,
        "details": {
            "selected_tools": [tc.tool_name for tc in trace.tool_calls],
            "num_tool_calls": len(trace.tool_calls),
            "errors": [tc.error for tc in trace.tool_calls if tc.error],
        },
    }
