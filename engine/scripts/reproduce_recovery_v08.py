#!/usr/bin/env python3
"""Portable launcher. Original results are never overwritten.

Examples:
    python scripts/reproduce_recovery_v08.py smoke --out rerun_smoke
    python scripts/reproduce_recovery_v08.py all --out rerun_all --workers 4
    python scripts/reproduce_recovery_v08.py replay --out replay_check
    python scripts/reproduce_recovery_v08.py references --out reference_check

Go must be on PATH. Core code uses no external Go modules. Smoke/replay/reference
launcher uses Python standard library; descriptive analysis additionally uses NumPy.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_SUITE = ROOT / "results/recovery_v08"


def run_logged(command: list[str], log: Path, timeout: int = 900) -> None:
    """Record stderr/stdout and fail explicitly; never silently skip a failed job."""
    completed = subprocess.run(
        command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        env={**os.environ, "GOMAXPROCS": "2"}, timeout=timeout, check=False,
    )
    log.parent.mkdir(parents=True, exist_ok=True)
    log.write_bytes(completed.stdout)
    if completed.returncode:
        raise RuntimeError(f"Exit {completed.returncode}; inspect {log}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("smoke", "regenerate20", "all", "replay", "references"))
    parser.add_argument("--out", type=Path, required=True, help="A new directory; must not exist")
    parser.add_argument("--suite", type=Path, default=DEFAULT_SUITE)
    parser.add_argument("--workers", type=int, default=4)
    args = parser.parse_args()
    if not 1 <= args.workers <= 16:
        parser.error("--workers must be between 1 and 16")
    target, suite = args.out.resolve(), args.suite.resolve()
    if target.exists():
        parser.error("Output exists; choose another directory to preserve original data")
    plan_path = suite / "plan.json"
    if not plan_path.is_file():
        parser.error(f"Missing plan: {plan_path}")
    go = shutil.which("go")
    if go is None:
        parser.error("Go is not on PATH")
    jobs = json.loads(plan_path.read_text(encoding="utf-8"))
    if len(jobs) != 240:
        parser.error("This reproduction launcher expects the archived 240-job DEV-R08 plan")
    if args.mode in ("replay", "references") and not (suite / "episodes/ep000/controller.jsonl.gz").exists():
        parser.error("Raw episode logs are missing. Use the full package or generate the series first")
    target.mkdir(parents=True)
    binary = target / ("recoverycheck.exe" if os.name == "nt" else "recoverycheck")
    run_logged([go, "build", "-o", str(binary), "./cmd/recoverycheck"], target / "build.log")
    entries: list[dict] = []
    if args.mode == "all":
        generated = target / "series"
        generated.mkdir()
        shutil.copyfile(plan_path, generated / "plan.json")
        # The original, frozen runner accepts an explicit binary and does not
        # rely on its historical default /mnt/data path in this invocation.
        run_logged([
            sys.executable, str(ROOT / "scripts/run_recovery_v08.py"),
            "--plan", str(generated / "plan.json"), "--dir", str(generated),
            "--binary", str(binary), "--workers", str(args.workers),
        ], target / "all_jobs.log", timeout=7200)
        entries.append({"generated_suite": str(generated), "jobs": 240})
    elif args.mode == "references":
        references = []
        for index, job in enumerate(jobs):
            if job["arm"] != "H" or job["profile"] == "normal" or job["repeat"] > 1:
                continue
            p = {"S": 1, "C": 2, "D": 3, "DS": 4}[job["profile"]]
            for day in (19, 20, 22):
                references.append((index, day, 1010001 + 1000*p + 100*job["repeat"] + day))
        for index, day, seed in references:
            folder = target / f"ep{index:03d}_day{day:02d}"
            run_logged([
                str(binary), "-reference", str(suite / "episodes" / f"ep{index:03d}"),
                "-day", str(day), "-seed", str(seed), "-paths", "2048", "-out", str(folder),
            ], target / f"ep{index:03d}_day{day:02d}.log")
            entries.append({"job": index, "day": day, "skipped": (folder / "SKIPPED.json").exists()})
    else:
        if args.mode == "smoke":
            indices = [i for i,j in enumerate(jobs) if j["profile"] == "S" and j["repeat"] == 0]
        elif args.mode == "regenerate20":
            indices = [i for i,j in enumerate(jobs) if j["repeat"] == 0]
        else:
            indices = list(range(len(jobs)))
        for index in indices:
            archived = suite / "episodes" / f"ep{index:03d}"
            output = target / f"ep{index:03d}"
            command = [str(binary), "-replay", str(archived)] if args.mode == "replay" else [
                str(binary), "-plan", str(plan_path), "-job", str(index), "-out", str(output)]
            run_logged(command, target / f"ep{index:03d}.log")
            item = {"job": index, "passed": True}
            if args.mode != "replay":
                hashes = {}
                for name in ("experiment_config.json", "controller.jsonl.gz", "physical.jsonl.gz", "summary.json", "COMPLETED.json"):
                    data = (output / name).read_bytes()
                    hashes[name] = hashlib.sha256(data).hexdigest()
                    if (archived / name).exists() and data != (archived / name).read_bytes():
                        raise RuntimeError(f"Not bitwise equal: job {index} {name}. Check environment/version.")
                item["files"] = hashes
                item["compared_with_archived_raw_files"] = (archived / "controller.jsonl.gz").exists()
            entries.append(item)
    report = {"mode": args.mode, "source_plan_sha256": hashlib.sha256(plan_path.read_bytes()).hexdigest(),
              "completed": True, "entries": entries}
    (target / "REPRODUCTION_COMPLETED.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Completed {args.mode}; logs and report: {target}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"Reproduction failed: {error}", file=sys.stderr)
        raise SystemExit(1)
