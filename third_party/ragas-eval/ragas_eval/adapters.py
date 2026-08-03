"""Adapters for importing Go agent trace data and converting to ragas types.

The kratos-base agent runtime produces traces via datatrace.DelegationTraceStore.
These traces contain DelegationEvent records with tool call details.

This module provides:
1. Parse Go trace JSON exports → AgentTrace (our internal model)
2. Convert AgentTrace + EvalCase → ragas MultiTurnSample (for ragas metrics)
3. Manual trace creation for ad-hoc evaluation
"""

import json
from pathlib import Path
from typing import Any, Dict, List, Optional, Union

from ragas.dataset_schema import MultiTurnSample
from ragas.messages import AIMessage, ToolCall

from .dataset import EvalCase
from .metrics import AgentTrace, ToolCallRecord


# ── Load traces from Go exports ──────────────────────────────────────────────

def load_traces_from_dir(trace_dir: Union[Path, str]) -> List[AgentTrace]:
    """Load all JSON trace files from a directory."""
    trace_dir = Path(trace_dir)
    traces: List[AgentTrace] = []

    if not trace_dir.exists():
        return traces

    jsonl_path = trace_dir / "traces.jsonl"
    if jsonl_path.exists():
        return load_trace_from_jsonl(jsonl_path)

    for json_file in sorted(trace_dir.glob("*.json")):
        with open(json_file, encoding="utf-8") as f:
            raw = json.load(f)
            trace = _parse_trace(raw)
            if trace:
                traces.append(trace)

    return traces


def load_trace_from_jsonl(jsonl_path: Union[Path, str]) -> List[AgentTrace]:
    """Load traces from a single JSONL file."""
    traces: List[AgentTrace] = []
    with open(jsonl_path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                raw = json.loads(line)
                trace = _parse_trace(raw)
                if trace:
                    traces.append(trace)
            except (json.JSONDecodeError, KeyError, TypeError):
                continue
    return traces


def _parse_trace(raw: Dict[str, Any]) -> Optional[AgentTrace]:
    """Parse a raw trace dict into an AgentTrace."""
    task_id = raw.get("task_id", "")
    if not task_id:
        return None

    tool_calls: List[ToolCallRecord] = []
    if "tool_calls" in raw:
        for i, tc in enumerate(raw["tool_calls"]):
            tool_calls.append(ToolCallRecord(
                tool_name=tc.get("tool_name", ""),
                input=tc.get("input", ""),
                output=tc.get("output"),
                error=tc.get("error"),
                exit_code=tc.get("exit_code"),
                duration_ms=tc.get("duration_ms"),
                step_index=i,
            ))
    elif "events" in raw:
        tool_events = [e for e in raw["events"] if e.get("stage", "").startswith("tool_")]
        for i, event in enumerate(tool_events):
            tool_calls.append(ToolCallRecord(
                tool_name=event.get("tool_name", ""),
                input=event.get("tool_input", ""),
                output=event.get("tool_output"),
                error=event.get("error"),
                exit_code=event.get("exit_code"),
                duration_ms=event.get("duration_ms"),
                step_index=i,
            ))

    return AgentTrace(
        task_id=task_id,
        user_query=raw.get("user_query", ""),
        tool_calls=tool_calls,
        final_response=raw.get("final_response"),
        metadata=raw.get("metadata", {}),
    )


def create_trace_from_manual(
    task_id: str,
    user_query: str,
    tool_calls: List[Dict[str, Any]],
    final_response: Optional[str] = None,
) -> AgentTrace:
    """Create an AgentTrace manually from recorded observations."""
    records: List[ToolCallRecord] = []
    for i, tc in enumerate(tool_calls):
        records.append(ToolCallRecord(
            tool_name=tc["tool_name"],
            input=tc.get("input", ""),
            output=tc.get("output"),
            error=tc.get("error"),
            exit_code=tc.get("exit_code"),
            duration_ms=tc.get("duration_ms"),
            step_index=i,
        ))
    return AgentTrace(
        task_id=task_id,
        user_query=user_query,
        tool_calls=records,
        final_response=final_response,
    )


# ── Convert to ragas types ───────────────────────────────────────────────────

def trace_to_ragas_sample(trace: AgentTrace) -> MultiTurnSample:
    """Convert an AgentTrace to a ragas MultiTurnSample.

    The trace's tool calls become AIMessage.tool_calls for ragas consumption.
    """
    ragas_tool_calls: List[ToolCall] = []
    for tc in trace.tool_calls:
        params = _parse_tool_params(tc.input)
        ragas_tool_calls.append(ToolCall(name=tc.tool_name, args=params))

    user_input = [
        AIMessage(content=trace.user_query, tool_calls=ragas_tool_calls)
    ] if ragas_tool_calls else [
        AIMessage(content=trace.user_query, tool_calls=None)
    ]

    return MultiTurnSample(user_input=user_input, reference_tool_calls=None)


def case_to_reference_tool_calls(case: EvalCase) -> List[ToolCall]:
    """Convert an EvalCase's expected tool expectations to ragas ToolCall list.

    For multi_step cases, returns the ordered list of expected tool calls.
    For cases with expected_params but multiple expected_tools (e.g. read_file + edit_file),
        includes ALL expected tools, carrying params only for the matched tool.
    For tool_selection cases, returns tools with empty args.
    """
    ref_calls: List[ToolCall] = []

    if case.multi_step is not None:
        for step in case.multi_step.steps:
            ref_calls.append(ToolCall(name=step.tool_name, args=step.params))
    elif case.expected_params is not None:
        ep_tool = case.expected_params.tool_name
        ep_params = case.expected_params.params
        # Build reference with all expected tools, params only on the matched one
        for tool_name in case.expected_tools:
            args = ep_params if tool_name == ep_tool else {}
            ref_calls.append(ToolCall(name=tool_name, args=args))
    else:
        for tool_name in case.expected_tools:
            ref_calls.append(ToolCall(name=tool_name, args={}))

    return ref_calls


def build_ragas_sample(case: EvalCase, trace: AgentTrace) -> MultiTurnSample:
    """Build a complete ragas MultiTurnSample from a case + trace pair.

    Strategy by category:
    - tool_selection: compare tool names only (empty args on both sides)
    - error_recovery: compare tool names only (args don't matter for errors)
    - param_extraction: full name+args comparison (ragas ExactMatch)
    - multi_step: full name+args with strict_order
    """
    name_only = case.category in ("tool_selection", "error_recovery")

    # Actual tool calls from trace
    actual_calls: List[ToolCall] = []
    for tc in trace.tool_calls:
        params = _parse_tool_params(tc.input) if not name_only else {}
        actual_calls.append(ToolCall(name=tc.tool_name, args=params))

    # Reference tool calls from case
    ref_calls = case_to_reference_tool_calls(case)
    if name_only:
        # Strip args for name-only comparison
        ref_calls = [ToolCall(name=r.name, args={}) for r in ref_calls]

    # user_input carries the conversation with embedded tool calls
    messages: List[AIMessage] = []
    if actual_calls:
        messages.append(AIMessage(content=case.user_query, tool_calls=actual_calls))
    else:
        messages.append(AIMessage(content=case.user_query, tool_calls=None))

    return MultiTurnSample(user_input=messages, reference_tool_calls=ref_calls)


def _parse_tool_params(input_str: str) -> Dict[str, Any]:
    """Parse tool input string to a dict of params.

    read_file accepts both "README.md" (bare path) and {"path": "README.md"} (JSON).
    exec_command accepts both "go test" and {"command": "go test"}.
    """
    input_str = input_str.strip()
    if not input_str:
        return {}

    if input_str.startswith("{"):
        try:
            return json.loads(input_str)
        except json.JSONDecodeError:
            return {"input": input_str}

    # Bare string — wrap as "input" key to match our tool JSON schema
    return {"input": input_str}
