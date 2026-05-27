import glob
import json
import logging
import os
import subprocess
from pathlib import Path
from typing import Optional

# All logs go to /logs per project convention; fall back to repo-level logs/ in dev
_log_dir = "/logs" if os.path.exists("/logs") else os.path.join(os.path.dirname(__file__), "..", "logs")
os.makedirs(_log_dir, exist_ok=True)

logging.basicConfig(
    filename=os.path.join(_log_dir, "lean_runner.log"),
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "service": "lean_runner", "level": "%(levelname)s", "message": "%(message)s"}'
)
logger = logging.getLogger(__name__)

LEAN_IMAGE = os.environ.get("LEAN_IMAGE", "lean-atp:latest")


def _find_results_json(job_dir: str) -> Optional[str]:
    """Find the LEAN results JSON file in the Results subdirectory."""
    pattern = os.path.join(job_dir, "Results", "*.json")
    matches = glob.glob(pattern)
    # Prefer *-summary.json if present
    summary = [m for m in matches if m.endswith("-summary.json")]
    if summary:
        return summary[0]
    return matches[0] if matches else None


def run_lean_backtest(job_id: str, job_dir: str, timeout_seconds: int = 7200) -> str:
    """
    Run LEAN in backtest mode. Blocks until completion or timeout.

    Returns path to results JSON file.
    Raises TimeoutError if container exceeds timeout_seconds.
    Raises RuntimeError if container exits with non-zero code.
    """
    logger.info(f"Starting backtest container for job {job_id}")

    cmd = [
        "docker", "run", "--rm",
        "-v", f"{os.path.abspath(job_dir)}:/lean",
        LEAN_IMAGE
    ]

    try:
        result = subprocess.run(
            cmd,
            timeout=timeout_seconds,
            capture_output=True,
            text=True
        )
    except subprocess.TimeoutExpired:
        logger.error(f"Backtest job {job_id} timed out after {timeout_seconds}s")
        # Kill the container
        subprocess.run(
            ["docker", "ps", "-q", "--filter", f"ancestor={LEAN_IMAGE}"],
            capture_output=True, text=True
        )
        raise TimeoutError(f"LEAN backtest exceeded {timeout_seconds}s timeout")

    if result.returncode != 0:
        logger.error(f"Backtest job {job_id} failed with exit code {result.returncode}: {result.stderr[:500]}")
        raise RuntimeError(f"LEAN container exited with code {result.returncode}: {result.stderr[:500]}")

    results_path = _find_results_json(job_dir)
    if not results_path:
        raise RuntimeError(f"LEAN completed but no results JSON found in {job_dir}/Results/")

    logger.info(f"Backtest job {job_id} completed. Results at {results_path}")
    return results_path


def run_lean_live(job_id: str, job_dir: str) -> str:
    """
    Start LEAN in live trading mode. Returns immediately with container ID.

    Returns container ID string.
    """
    logger.info(f"Starting live trading container for job {job_id}")

    cmd = [
        "docker", "run", "-d", "--rm",
        "-v", f"{os.path.abspath(job_dir)}:/lean",
        "--add-host=host.docker.internal:host-gateway",
        LEAN_IMAGE
    ]

    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"Failed to start live LEAN container: {result.stderr}")

    container_id = result.stdout.strip()
    logger.info(f"Live job {job_id} started in container {container_id}")
    return container_id


def stop_lean_live(container_id: str, job_dir: str) -> Optional[str]:
    """
    Stop a running live LEAN container gracefully.

    Returns path to results JSON if available, else None.
    """
    logger.info(f"Stopping live container {container_id}")

    subprocess.run(
        ["docker", "stop", "--time", "30", container_id],
        capture_output=True, text=True
    )

    return _find_results_json(job_dir)


def is_container_running(container_id: str) -> bool:
    """Check if a Docker container is still running."""
    result = subprocess.run(
        ["docker", "inspect", "--format", "{{.State.Running}}", container_id],
        capture_output=True, text=True
    )
    return result.stdout.strip() == "true"


def poll_live_results(job_dir: str) -> Optional[dict]:
    """
    Read and parse the live results JSON if it exists.
    Returns parsed dict or None if file doesn't exist yet.
    """
    results_path = _find_results_json(job_dir)
    if not results_path:
        return None
    try:
        with open(results_path) as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError):
        return None
