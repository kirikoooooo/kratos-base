"""Evaluation report generation — JSONL, JSON summary, and console output."""

import csv
import json
from pathlib import Path
from typing import Any, Dict, Set

from .dataset import EvaluationReport


class EvaluationReporter:
    """Saves evaluation results to structured output files."""

    def __init__(self, output_dir: Path) -> None:
        self.output_dir = Path(output_dir)

    def save(self, report: EvaluationReport) -> None:
        """Save full evaluation report to the output directory."""
        run_dir = self.output_dir / report.config.run_id
        run_dir.mkdir(parents=True, exist_ok=True)

        # Per-case results (JSONL)
        cases_path = run_dir / "per_case.jsonl"
        with open(cases_path, "w", encoding="utf-8") as f:
            for result in report.per_case:
                f.write(json.dumps(result, ensure_ascii=False) + "\n")

        # Judge results (JSONL)
        if report.judge_results:
            judge_path = run_dir / "judge_results.jsonl"
            with open(judge_path, "w", encoding="utf-8") as f:
                for result in report.judge_results:
                    f.write(json.dumps(result, ensure_ascii=False) + "\n")

        # Summary (JSON)
        summary_path = run_dir / "summary.json"
        with open(summary_path, "w", encoding="utf-8") as f:
            json.dump(
                {
                    "timestamp": report.timestamp,
                    "config": {
                        "dataset": str(report.config.dataset_path),
                        "run_id": report.config.run_id,
                        "use_llm_judge": report.config.use_llm_judge,
                        "judge_model": report.config.judge_model,
                        "embedding_model": report.config.embedding_model,
                        "llm_model": report.config.llm_model,
                        "prompt_version": report.config.prompt_version,
                        "index_hash": report.config.index_hash,
                        "random_seed": report.config.random_seed,
                        "tags": report.config.tags,
                    },
                    "summary": report.summary,
                },
                f,
                ensure_ascii=False,
                indent=2,
            )

        # CSV for spreadsheet analysis
        self._save_csv(report, run_dir / "metrics.csv")

        # Failure report
        self._save_failures(report, run_dir / "failures.jsonl")

    def _save_csv(self, report: EvaluationReport, path: Path) -> None:
        """Save per-case metrics as CSV."""
        if not report.per_case:
            return

        # Collect all metric names across cases
        all_metrics: Set[str] = set()
        for result in report.per_case:
            all_metrics.update(result.get("metrics", {}).keys())

        metric_names = sorted(all_metrics)
        fieldnames = ["case_id", "category", "passed"] + metric_names + ["errors"]

        with open(path, "w", newline="", encoding="utf-8") as f:
            writer = csv.DictWriter(f, fieldnames=fieldnames, extrasaction="ignore")
            writer.writeheader()

            for result in report.per_case:
                row: Dict[str, Any] = {
                    "case_id": result["case_id"],
                    "category": result["category"],
                    "passed": result["passed"],
                }
                row.update(result.get("metrics", {}))
                details = result.get("details", {})
                row["errors"] = "; ".join(details.get("errors", []))

                writer.writerow(row)

    def _save_failures(self, report: EvaluationReport, path: Path) -> None:
        """Save failed cases for debugging."""
        failures = [r for r in report.per_case if not r.get("passed", False)]

        with open(path, "w", encoding="utf-8") as f:
            for failure in failures:
                f.write(json.dumps(failure, ensure_ascii=False) + "\n")
