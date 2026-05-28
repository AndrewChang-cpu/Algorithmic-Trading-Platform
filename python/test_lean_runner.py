import json
import os
import subprocess
from unittest.mock import MagicMock, call, patch

import pytest

os.environ["LEAN_IMAGE"] = "lean-atp:latest"

import lean_runner


def test_run_lean_backtest_success(tmp_path):
    results_dir = tmp_path / "Results"
    results_dir.mkdir()
    results_file = results_dir / "result.json"
    results_file.write_text("{}")

    mock_result = MagicMock(returncode=0, stdout="", stderr="")
    with patch("lean_runner.subprocess.run", return_value=mock_result):
        path = lean_runner.run_lean_backtest("job-1", str(tmp_path))

    assert path.endswith(".json")
    assert os.path.exists(path)


def test_run_lean_backtest_timeout(tmp_path):
    def side_effect(cmd, **kwargs):
        if cmd[0:2] == ["docker", "run"]:
            raise subprocess.TimeoutExpired(cmd=cmd, timeout=1)
        return MagicMock(returncode=0)

    with patch("lean_runner.subprocess.run", side_effect=side_effect):
        with pytest.raises(TimeoutError):
            lean_runner.run_lean_backtest("job-2", str(tmp_path), timeout_seconds=1)


def test_run_lean_backtest_nonzero_exit(tmp_path):
    mock_result = MagicMock(returncode=1, stderr="something went wrong")
    with patch("lean_runner.subprocess.run", return_value=mock_result):
        with pytest.raises(RuntimeError):
            lean_runner.run_lean_backtest("job-3", str(tmp_path))


def test_run_lean_live_returns_container_id(tmp_path):
    mock_result = MagicMock(returncode=0, stdout="abc123def456\n")
    with patch("lean_runner.subprocess.run", return_value=mock_result):
        container_id = lean_runner.run_lean_live("job-4", str(tmp_path))

    assert container_id == "abc123def456"


def test_stop_lean_live(tmp_path):
    results_dir = tmp_path / "Results"
    results_dir.mkdir()
    results_file = results_dir / "final-summary.json"
    results_file.write_text(json.dumps({"state": "done"}))

    mock_result = MagicMock(returncode=0)
    with patch("lean_runner.subprocess.run", return_value=mock_result) as mock_run:
        path = lean_runner.stop_lean_live("some-id", str(tmp_path))

    assert path is not None
    assert path.endswith(".json")
    mock_run.assert_called_once_with(
        ["docker", "stop", "--time", "30", "some-id"],
        capture_output=True,
        text=True,
        check=True,
    )


def test_poll_live_results_none(tmp_path):
    result = lean_runner.poll_live_results(str(tmp_path))
    assert result is None


def test_poll_live_results_returns_dict(tmp_path):
    results_dir = tmp_path / "Results"
    results_dir.mkdir()
    results_file = results_dir / "snapshot.json"
    results_file.write_text(json.dumps({"equity": 10000}))

    result = lean_runner.poll_live_results(str(tmp_path))
    assert result == {"equity": 10000}


def test_timeout_kills_container(tmp_path):
    """Timeout handler must find the container by job label and docker kill it."""
    container_id = "deadbeef1234"

    def side_effect(cmd, **kwargs):
        if cmd[0:2] == ["docker", "run"]:
            raise subprocess.TimeoutExpired(cmd=cmd, timeout=1)
        if cmd[0:4] == ["docker", "ps", "-q", "--filter"]:
            return MagicMock(returncode=0, stdout=f"{container_id}\n")
        # docker kill
        return MagicMock(returncode=0, stdout="")

    with patch("lean_runner.subprocess.run", side_effect=side_effect) as mock_run:
        with pytest.raises(TimeoutError):
            lean_runner.run_lean_backtest("job-timeout", str(tmp_path), timeout_seconds=1)

    kill_calls = [c for c in mock_run.call_args_list if c.args[0][0:2] == ["docker", "kill"]]
    assert len(kill_calls) == 1
    assert kill_calls[0].args[0] == ["docker", "kill", container_id]


def test_stop_lean_live_raises_on_failure(tmp_path):
    """stop_lean_live must propagate CalledProcessError when docker stop fails."""
    with patch(
        "lean_runner.subprocess.run",
        side_effect=subprocess.CalledProcessError(1, "docker stop"),
    ):
        with pytest.raises(subprocess.CalledProcessError):
            lean_runner.stop_lean_live("bad-container-id", str(tmp_path))
