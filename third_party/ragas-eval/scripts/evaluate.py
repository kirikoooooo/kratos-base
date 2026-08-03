#!/usr/bin/env python3
"""CLI entry point for kratos-base tool-calling evaluation.

Usage:
    # Evaluate tool selection
    python scripts/evaluate.py --dataset datasets/tool_selection.jsonl --traces .myagent/traces/

    # Evaluate with LLM judge
    python scripts/evaluate.py --dataset datasets/tool_selection.jsonl --traces traces.jsonl --judge

    # Evaluate all datasets
    python scripts/evaluate.py --all --traces .myagent/traces/

Environment variables:
    OPENAI_API_KEY   — required when using --judge
    OPENAI_BASE_URL  — optional custom endpoint
    JUDGE_MODEL      — override judge model (default: gpt-4o-mini)
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

# Add project root to path
_project_root = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(_project_root))

from rich.console import Console

from ragas_eval.adapters import load_traces_from_dir, load_trace_from_jsonl
from ragas_eval.dataset import EvalDataset
from ragas_eval.evaluator import EvalConfig, ToolCallEvaluator


def main() -> None:
    console = Console()

    parser = argparse.ArgumentParser(
        description="Evaluate kratos-base agent tool-calling capabilities with ragas-inspired metrics",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python scripts/evaluate.py -d datasets/tool_selection.jsonl -t .myagent/traces/
  python scripts/evaluate.py -d datasets/tool_selection.jsonl -t traces.jsonl --judge
  python scripts/evaluate.py --all -t .myagent/traces/ -o .myagent/rag/evaluations/
        """,
    )

    parser.add_argument(
        "-d", "--dataset",
        type=str,
        help="Path to evaluation dataset (JSONL)",
    )
    parser.add_argument(
        "--all",
        action="store_true",
        help="Evaluate all datasets in datasets/ directory",
    )
    parser.add_argument(
        "-t", "--traces",
        type=str,
        default="",
        help="Path to trace directory or JSONL file with agent traces",
    )
    parser.add_argument(
        "-o", "--output",
        type=str,
        default=".myagent/rag/evaluations",
        help="Output directory for evaluation reports",
    )
    parser.add_argument(
        "--judge",
        action="store_true",
        help="Enable LLM-as-a-judge qualitative evaluation",
    )
    parser.add_argument(
        "--judge-model",
        type=str,
        default="gpt-4o-mini",
        help="LLM model for judge (default: gpt-4o-mini)",
    )
    parser.add_argument(
        "--threshold",
        type=float,
        default=0.7,
        help="Pass/fail threshold (default: 0.7)",
    )
    parser.add_argument(
        "--run-id",
        type=str,
        default=None,
        help="Custom run ID for this evaluation",
    )
    parser.add_argument(
        "--embedding-model",
        type=str,
        default="",
        help="Embedding model used (for reproducibility)",
    )
    parser.add_argument(
        "--llm-model",
        type=str,
        default="",
        help="LLM model used by the agent (for reproducibility)",
    )
    parser.add_argument(
        "--list-datasets",
        action="store_true",
        help="List available datasets and exit",
    )

    args = parser.parse_args()

    datasets_dir = _project_root / "datasets"

    # List datasets
    if args.list_datasets:
        console.print("[bold]Available datasets:[/bold]")
        for ds in sorted(datasets_dir.glob("*.jsonl")):
            try:
                d = EvalDataset(ds)
                d.load()
                summary = d.summary()
                total = sum(summary.values())
                console.print(f"  {ds.name}: {total} cases ({', '.join(f'{k}: {v}' for k, v in summary.items())})")
            except Exception as e:
                console.print(f"  {ds.name}: [red]error: {e}[/red]")
        return

    # Determine datasets to evaluate
    if args.all:
        dataset_paths = sorted(datasets_dir.glob("*.jsonl"))
    elif args.dataset:
        dataset_paths = [Path(args.dataset)]
    else:
        console.print("[red]Error:[/red] specify --dataset or --all")
        sys.exit(1)

    if not args.traces:
        console.print("[red]Error:[/red] specify --traces (path to trace JSONL/directory)")
        sys.exit(1)

    # Load traces
    traces_path = Path(args.traces)
    if traces_path.is_file():
        traces = load_trace_from_jsonl(traces_path)
    elif traces_path.is_dir():
        traces = load_traces_from_dir(traces_path)
    else:
        console.print(f"[red]Error:[/red] traces path not found: {args.traces}")
        sys.exit(1)

    console.print(f"[dim]Loaded {len(traces)} traces from {args.traces}[/dim]")

    if not traces:
        console.print("[yellow]Warning:[/yellow] no traces loaded. Create trace files first.")
        console.print("[dim]Tip: run the agent with --trace enabled to generate trace data.[/dim]")

    # Evaluate each dataset
    all_reports = []
    for ds_path in dataset_paths:
        if not ds_path.exists():
            console.print(f"[yellow]Skipping missing dataset: {ds_path}[/yellow]")
            continue

        console.print(f"\n[bold]━━━ Evaluating {ds_path.name} ━━━[/bold]")

        dataset = EvalDataset(ds_path)
        dataset.load()

        config = EvalConfig(
            dataset_path=ds_path.resolve(),
            output_dir=Path(args.output),
            run_id=args.run_id or "",
            use_llm_judge=args.judge,
            judge_model=args.judge_model,
            pass_threshold=args.threshold,
            embedding_model=args.embedding_model,
            llm_model=args.llm_model,
        )

        evaluator = ToolCallEvaluator(config, console=console)
        report = evaluator.evaluate(dataset, traces)
        evaluator._print_summary(report)
        all_reports.append(report)

    # Merge and save all reports in one shot
    if all_reports:
        from ragas_eval.evaluator import EvaluationReport
        from ragas_eval.reporter import EvaluationReporter

        merged = EvaluationReport(config=EvalConfig(
            dataset_path=Path(args.output).resolve(),
            output_dir=Path(args.output),
            run_id=args.run_id or "",
            use_llm_judge=args.judge,
            judge_model=args.judge_model,
            pass_threshold=args.threshold,
            embedding_model=args.embedding_model,
            llm_model=args.llm_model,
        ))
        merged.per_case = [c for r in all_reports for c in r.per_case]
        merged.judge_results = [j for r in all_reports for j in r.judge_results]
        total = len(merged.per_case)
        passed = sum(1 for c in merged.per_case if c.get("passed", False))
        merged.summary = {
            "total_cases": total,
            "passed": passed,
            "failed": total - passed,
            "pass_rate": passed / total if total > 0 else 0.0,
            "by_category": {},
        }
        for c in merged.per_case:
            cat = c.get("category", "unknown")
            if cat not in merged.summary["by_category"]:
                merged.summary["by_category"][cat] = {"total": 0, "passed": 0}
            merged.summary["by_category"][cat]["total"] += 1
            if c.get("passed", False):
                merged.summary["by_category"][cat]["passed"] += 1

        reporter = EvaluationReporter(Path(args.output))
        reporter.save(merged)

    # Final summary
    if len(all_reports) > 1:
        console.print("\n[bold]═══ Overall Summary ═══[/bold]")
        total_cases = sum(r.summary["total_cases"] for r in all_reports)
        total_passed = sum(r.summary["passed"] for r in all_reports)
        overall_rate = total_passed / total_cases if total_cases > 0 else 0
        console.print(f"  Total: {total_cases} cases, {total_passed} passed ({overall_rate:.1%})")
        console.print(f"  Reports saved to: {args.output}")


if __name__ == "__main__":
    main()
