#!/usr/bin/env python3
"""Run the shared matched batch harness for the setup-consolidation experiment."""
import importlib.util
import os
from pathlib import Path

here = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("matched_batches", here / "run-deadline.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
runner.BEFORE = Path(os.environ.get("SETUP_BEFORE_SOURCE", "/private/tmp/jsontrans-before-setup"))
runner.OUT = here / "testdata/results/setup"
runner.LABEL = "Batch setup"
if __name__ == "__main__":
    runner.main()
