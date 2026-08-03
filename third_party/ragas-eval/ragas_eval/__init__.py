"""ragas-eval: Evaluation framework for kratos-base agent tool-calling capabilities.

Powered by ragas official metrics:
- ragas.metrics.collections.ToolCallAccuracy
- ragas.metrics.collections.ToolCallF1

With supplementary metrics for dimensions ragas does not cover
(multi-step compliance via LCS, error recovery scoring).
"""

from .adapters import create_trace_from_manual, load_traces_from_dir, load_trace_from_jsonl
from .dataset import (
    EvalCase,
    EvalConfig,
    EvalDataset,
    EvalResult,
    EvaluationReport,
    MultiStepSequence,
    ToolExpectation,
)
from .metrics import AgentTrace, ToolCallRecord, evaluate_case
from .reporter import EvaluationReporter

__all__ = [
    "AgentTrace",
    "ToolCallRecord",
    "EvalCase",
    "EvalConfig",
    "EvalDataset",
    "EvalResult",
    "EvaluationReport",
    "EvaluationReporter",
    "MultiStepSequence",
    "ToolExpectation",
    "ToolCallEvaluator",
    "LLMJudge",
    "evaluate_case",
    "create_trace_from_manual",
    "load_traces_from_dir",
    "load_trace_from_jsonl",
]


def __getattr__(name: str):
    """Lazy import for modules with optional dependencies."""
    if name == "ToolCallEvaluator":
        from . import evaluator as _m
        return getattr(_m, name)
    if name == "LLMJudge":
        from . import judge as _m
        return getattr(_m, name)
    raise AttributeError(f"module 'ragas_eval' has no attribute {name!r}")
