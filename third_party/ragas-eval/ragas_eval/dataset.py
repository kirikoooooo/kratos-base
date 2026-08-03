"""Evaluation dataset management for kratos-base tool-calling.

Each dataset record captures a user query and the expected tool-calling behaviour,
enabling offline evaluation of agent tool selection and parameter extraction accuracy.
"""

import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Literal, Optional, Set, Union

from pydantic import BaseModel, Field


class ToolExpectation(BaseModel):
    """Expected tool call for a single step."""

    tool_name: str = Field(description="Expected tool name (e.g. read_file, edit_file, exec_command)")
    params: Dict[str, Any] = Field(
        default_factory=dict,
        description="Expected parameters. Substrings or regex patterns for flexible matching.",
    )
    params_mode: Literal["exact", "contains", "subset"] = Field(
        default="subset",
        description="How to match params: exact, contains (substring), or subset (key subset match)",
    )
    should_succeed: bool = Field(default=True, description="Whether this tool call is expected to succeed")
    failure_pattern: Optional[str] = Field(default=None, description="Expected error substring if should_succeed=False")


class MultiStepSequence(BaseModel):
    """Expected sequence of tool calls for multi-step tasks."""

    steps: List[ToolExpectation] = Field(description="Ordered expected tool calls")
    allow_extra_steps: bool = Field(default=True, description="Allow extra tool calls between expected steps")
    strict_order: bool = Field(default=True, description="Whether the order of steps must match")


class EvalCase(BaseModel):
    """A single evaluation case for tool-calling assessment."""

    id: str = Field(description="Stable case identifier")
    category: str = Field(description="Evaluation category: tool_selection, param_extraction, multi_step, error_recovery")
    user_query: str = Field(description="The user query that triggers tool calling")
    language: str = Field(default="zh", description="Query language: zh, en")
    tags: List[str] = Field(default_factory=list, description="Tags for filtering and grouping")

    # Tool selection: which tool(s) should be chosen
    expected_tools: List[str] = Field(
        default_factory=list,
        description="Expected tool names the agent should select (order-independent)",
    )
    forbidden_tools: List[str] = Field(
        default_factory=list,
        description="Tools the agent should NOT select for this query",
    )

    # Parameter accuracy
    expected_params: Optional[ToolExpectation] = Field(
        default=None,
        description="Expected tool and parameters for single-step tasks",
    )

    # Multi-step sequences
    multi_step: Optional[MultiStepSequence] = Field(
        default=None,
        description="Expected multi-step tool sequence",
    )

    # Ground truth
    ground_truth_context: Optional[str] = Field(
        default=None,
        description="Optional reference context the tool should have accessed",
    )
    ground_truth_answer: Optional[str] = Field(
        default=None,
        description="Optional reference answer for end-to-end evaluation",
    )

    # Metadata
    created_at: str = Field(
        default_factory=lambda: datetime.now(timezone.utc).isoformat(),
    )
    source: str = Field(default="manual", description="Data source: manual, synthetic, trace_replay")

    def stable_hash(self) -> str:
        """Compute a stable hash for deduplication."""
        content = f"{self.category}|{self.user_query}|{','.join(sorted(self.expected_tools))}"
        return hashlib.sha256(content.encode()).hexdigest()[:12]


class EvalResult(BaseModel):
    """Result of evaluating one case."""

    case_id: str
    category: str
    passed: bool
    metrics: Dict[str, float] = Field(default_factory=dict)
    details: Dict[str, Any] = Field(default_factory=dict)
    errors: List[str] = Field(default_factory=list)


# ── Evaluation configuration and report types (no heavy deps) ───────────────

from dataclasses import dataclass, field as dc_field


@dataclass
class EvalConfig:
    """Evaluation run configuration."""

    dataset_path: Path
    output_dir: Path = dc_field(default_factory=lambda: Path(".myagent/rag/evaluations"))
    run_id: str = dc_field(
        default_factory=lambda: datetime.now(timezone.utc).strftime("run-%Y%m%d-%H%M%S")
    )
    use_llm_judge: bool = False
    judge_model: str = "gpt-4o-mini"
    pass_threshold: float = 0.7
    embedding_model: str = ""
    llm_model: str = ""
    prompt_version: str = ""
    index_hash: str = ""
    random_seed: int = 42
    tags: List[str] = dc_field(default_factory=list)


@dataclass
class EvaluationReport:
    """Complete evaluation report."""

    config: EvalConfig
    timestamp: str = dc_field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
    summary: Dict[str, Any] = dc_field(default_factory=dict)
    per_case: List[Dict[str, Any]] = dc_field(default_factory=list)
    judge_results: List[Dict[str, Any]] = dc_field(default_factory=list)


class EvalDataset:
    """Manages loading, validation, and versioning of evaluation datasets."""

    def __init__(self, path: Union[Path, str]) -> None:
        self.path = Path(path)
        self.cases: List[EvalCase] = []

    def load(self) -> List[EvalCase]:
        """Load evaluation cases from a JSONL file."""
        if not self.path.exists():
            raise FileNotFoundError(f"Dataset not found: {self.path}")

        cases: List[EvalCase] = []
        with open(self.path, encoding="utf-8") as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                try:
                    raw = json.loads(line)
                    case = EvalCase(**raw)
                    cases.append(case)
                except Exception as e:
                    raise ValueError(f"Line {line_num} in {self.path}: {e}") from e

        self.cases = cases
        self._validate()
        return cases

    def _validate(self) -> None:
        """Validate dataset consistency."""
        seen_ids: Set[str] = set()
        for case in self.cases:
            if case.id in seen_ids:
                raise ValueError(f"Duplicate case id: {case.id}")
            seen_ids.add(case.id)

            if case.expected_params is not None and case.multi_step is not None:
                raise ValueError(
                    f"Case {case.id}: cannot have both expected_params and multi_step"
                )

    def by_category(self, category: str) -> List[EvalCase]:
        """Filter cases by category."""
        return [c for c in self.cases if c.category == category]

    def by_tag(self, tag: str) -> List[EvalCase]:
        """Filter cases by tag."""
        return [c for c in self.cases if tag in c.tags]

    @property
    def categories(self) -> List[str]:
        """List unique categories."""
        return sorted({c.category for c in self.cases})

    def summary(self) -> Dict[str, int]:
        """Category counts."""
        counts: Dict[str, int] = {}
        for case in self.cases:
            counts[case.category] = counts.get(case.category, 0) + 1
        return counts
