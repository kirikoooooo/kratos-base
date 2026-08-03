"""Main evaluation runner for kratos-base tool-calling capabilities.

Powered by ragas ToolCallAccuracy + ToolCallF1 for core metrics,
with supplementary metrics for dimensions ragas does not cover.
"""

from pathlib import Path
from typing import Any, Dict, List, Optional

from rich.console import Console
from rich.progress import Progress, SpinnerColumn, TextColumn
from rich.table import Table

from .dataset import EvalCase, EvalConfig, EvalDataset, EvalResult, EvaluationReport
from .judge import LLMJudge
from .metrics import AgentTrace, evaluate_case
from .reporter import EvaluationReporter


class ToolCallEvaluator:
    """Evaluates tool-calling capabilities using ragas + supplementary metrics."""

    def __init__(self, config: EvalConfig, console: Optional[Console] = None) -> None:
        self.config = config
        self.console = console or Console()
        self.judge: Optional[LLMJudge] = (
            LLMJudge(model=config.judge_model) if config.use_llm_judge else None
        )
        self.reporter = EvaluationReporter(config.output_dir)

    def evaluate(
        self,
        dataset: EvalDataset,
        traces: List[AgentTrace],
    ) -> EvaluationReport:
        """Run full evaluation pipeline."""
        trace_map: Dict[str, AgentTrace] = {t.task_id: t for t in traces}

        report = EvaluationReport(config=self.config)

        self.console.print(f"\n[bold]🔍 Evaluating tool-calling with ragas + ragas-eval[/bold]")
        self.console.print(f"  Dataset: {dataset.path}")
        self.console.print(f"  Cases: {len(dataset.cases)}")
        self.console.print(f"  Traces: {len(traces)}")
        self.console.print(f"  LLM Judge: {'on' if self.config.use_llm_judge else 'off'}")
        self.console.print()

        per_case_results: List[Dict[str, Any]] = []
        judge_results: List[Dict[str, Any]] = []

        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            console=self.console,
        ) as progress:
            task = progress.add_task("Evaluating...", total=len(dataset.cases))

            for case in dataset.cases:
                trace = self._find_trace(case, trace_map)

                if trace is None:
                    per_case_results.append({
                        "case_id": case.id,
                        "category": case.category,
                        "passed": False,
                        "error": "No matching trace found",
                        "metrics": {},
                    })
                    progress.update(task, advance=1)
                    continue

                result = evaluate_case(case, trace)
                per_case_results.append(result)

                if self.judge is not None:
                    try:
                        jr = self._run_judge(case, trace)
                        judge_results.append(jr)
                    except Exception as e:
                        judge_results.append({"case_id": case.id, "error": str(e)})

                progress.update(task, advance=1)

        report.per_case = per_case_results
        report.judge_results = judge_results
        report.summary = self._compute_summary(per_case_results, judge_results)
        return report

    def _find_trace(
        self, case: EvalCase, trace_map: Dict[str, AgentTrace]
    ) -> Optional[AgentTrace]:
        if case.id in trace_map:
            return trace_map[case.id]
        for trace in trace_map.values():
            if trace.user_query.strip() == case.user_query.strip():
                return trace
        return None

    def _run_judge(self, case: EvalCase, trace: AgentTrace) -> Dict[str, Any]:
        assert self.judge is not None
        results: Dict[str, Any] = {"case_id": case.id}
        try:
            results["tool_selection"] = self.judge.judge_tool_selection(case, trace)
        except Exception as e:
            results["tool_selection"] = {"error": str(e)}
        if case.expected_params is not None:
            try:
                results["params"] = self.judge.judge_params(case, trace)
            except Exception as e:
                results["params"] = {"error": str(e)}
        if case.multi_step is not None:
            try:
                results["multi_step"] = self.judge.judge_multi_step(case, trace)
            except Exception as e:
                results["multi_step"] = {"error": str(e)}
        has_errors = any(tc.error is not None for tc in trace.tool_calls)
        if has_errors:
            try:
                results["error_recovery"] = self.judge.judge_error_recovery(case, trace)
            except Exception as e:
                results["error_recovery"] = {"error": str(e)}
        return results

    def _compute_summary(
        self,
        per_case: List[Dict[str, Any]],
        judge_results: List[Dict[str, Any]],
    ) -> Dict[str, Any]:
        total = len(per_case)
        if total == 0:
            return {"total_cases": 0}

        passed = sum(1 for r in per_case if r.get("passed", False))

        metric_aggs: Dict[str, List[float]] = {}
        for r in per_case:
            for name, value in r.get("metrics", {}).items():
                metric_aggs.setdefault(name, []).append(value)

        metric_averages = {
            name: sum(vals) / len(vals) for name, vals in metric_aggs.items()
        }

        by_category: Dict[str, Dict[str, Any]] = {}
        for r in per_case:
            cat = r.get("category", "unknown")
            by_category.setdefault(cat, {"total": 0, "passed": 0})
            by_category[cat]["total"] += 1
            if r.get("passed", False):
                by_category[cat]["passed"] += 1

        judge_summary: Dict[str, Any] = {}
        if judge_results:
            judge_scores: List[float] = []
            for jr in judge_results:
                for key in ("tool_selection", "params", "multi_step", "error_recovery"):
                    if key in jr and isinstance(jr[key], dict):
                        score = jr[key].get("score")
                        if isinstance(score, (int, float)):
                            judge_scores.append(float(score))
            if judge_scores:
                judge_summary["avg_score"] = sum(judge_scores) / len(judge_scores)
                judge_summary["num_judged"] = len(judge_results)

        return {
            "total_cases": total,
            "passed": passed,
            "failed": total - passed,
            "pass_rate": passed / total if total > 0 else 0.0,
            "metric_averages": metric_averages,
            "by_category": by_category,
            "judge": judge_summary,
        }

    def save_report(self, report: EvaluationReport) -> None:
        self.reporter.save(report)

    def _print_summary(self, report: EvaluationReport) -> None:
        s = report.summary
        self.console.print()
        self.console.print("[bold]📊 Evaluation Results[/bold]")

        table = Table(title="Overview")
        table.add_column("Metric", style="cyan")
        table.add_column("Value", style="green")
        table.add_row("Total Cases", str(s.get("total_cases", 0)))
        table.add_row("Passed", str(s.get("passed", 0)))
        table.add_row("Failed", str(s.get("failed", 0)))
        rate = s.get("pass_rate", 0)
        table.add_row("Pass Rate", f"{rate:.1%}")
        self.console.print(table)

        if s.get("metric_averages"):
            mt = Table(title="Metric Averages")
            mt.add_column("Metric", style="cyan")
            mt.add_column("Score", style="green")
            for name, score in sorted(s["metric_averages"].items()):
                color = "green" if score >= self.config.pass_threshold else "red"
                mt.add_row(name, f"[{color}]{score:.3f}[/{color}]")
            self.console.print(mt)

        if s.get("by_category"):
            ct = Table(title="By Category")
            ct.add_column("Category", style="cyan")
            ct.add_column("Passed/Total", style="yellow")
            ct.add_column("Rate", style="green")
            for cat, stats in sorted(s["by_category"].items()):
                r = stats["passed"] / stats["total"] if stats["total"] > 0 else 0
                ct.add_row(cat, f"{stats['passed']}/{stats['total']}", f"{r:.1%}")
            self.console.print(ct)

        if s.get("judge") and s["judge"].get("avg_score") is not None:
            self.console.print(
                f"\n[bold]🤖 LLM Judge Avg Score:[/bold] "
                f"[green]{s['judge']['avg_score']:.3f}[/green] "
                f"({s['judge']['num_judged']} cases judged)"
            )
