import json
import os
import tempfile
from unittest.mock import MagicMock, patch

import pytest

# Set required env vars before importing lean_runner
os.environ.setdefault("S3_BUCKET", "test-bucket")
os.environ.setdefault("S3_ACCESS_KEY", "test-key")
os.environ.setdefault("S3_SECRET_KEY", "test-secret")
os.environ.setdefault("K8S_NAMESPACE", "default")

import lean_runner  # noqa: E402


def test_run_lean_backtest_success():
    with tempfile.TemporaryDirectory() as tmpdir:
        mock_job = MagicMock()
        mock_job.status.succeeded = 1
        mock_job.status.failed = 0

        with patch("lean_runner.k8s_client.BatchV1Api") as mock_batch, patch(
            "lean_runner.k8s_client.CoreV1Api"
        ), patch("lean_runner.upload_job_inputs"), patch(
            "lean_runner.download_job_results"
        ) as mock_download:

            mock_batch_api = MagicMock()
            mock_batch.return_value = mock_batch_api
            mock_batch_api.read_namespaced_job.return_value = mock_job

            def fake_download(job_id, dest_dir):
                os.makedirs(dest_dir, exist_ok=True)
                with open(os.path.join(dest_dir, "result-summary.json"), "w") as f:
                    json.dump({"state": {"Status": "Completed"}}, f)

            mock_download.side_effect = fake_download

            result = lean_runner.run_lean_backtest(
                "abcdef12-1234-1234-1234-123456789abc", tmpdir, timeout_seconds=60
            )

            assert result.endswith(".json")
            mock_batch_api.create_namespaced_job.assert_called_once()


def test_run_lean_backtest_timeout():
    with tempfile.TemporaryDirectory() as tmpdir:
        mock_job = MagicMock()
        mock_job.status.succeeded = 0
        mock_job.status.failed = 0

        with patch("lean_runner.k8s_client.BatchV1Api") as mock_batch, patch(
            "lean_runner.k8s_client.CoreV1Api"
        ), patch("lean_runner.upload_job_inputs"), patch("lean_runner.time.sleep"):

            mock_batch_api = MagicMock()
            mock_batch.return_value = mock_batch_api
            mock_batch_api.read_namespaced_job.return_value = mock_job

            import time

            original_time = time.time
            call_count = [0]

            def fake_time():
                call_count[0] += 1
                if call_count[0] > 3:
                    return original_time() + 10000  # simulate past deadline
                return original_time()

            with patch("lean_runner.time.time", side_effect=fake_time):
                with pytest.raises(TimeoutError):
                    lean_runner.run_lean_backtest(
                        "abcdef12-1234-1234-1234-123456789abc",
                        tmpdir,
                        timeout_seconds=1,
                    )

            mock_batch_api.delete_namespaced_job.assert_called_once()


def test_backtest_timeout_kills_all_containers():
    """Renamed from Docker version: verifies K8s job is deleted on timeout."""
    with tempfile.TemporaryDirectory() as tmpdir:
        mock_job = MagicMock()
        mock_job.status.succeeded = 0
        mock_job.status.failed = 0

        mock_delete_result = MagicMock()
        mock_delete_result.returncode = 1  # returncode=1 as specified in plan

        with patch("lean_runner.k8s_client.BatchV1Api") as mock_batch, patch(
            "lean_runner.k8s_client.CoreV1Api"
        ), patch("lean_runner.upload_job_inputs"), patch(
            "lean_runner.time.sleep"
        ), patch(
            "lean_runner.logger"
        ) as mock_logger:

            mock_batch_api = MagicMock()
            mock_batch.return_value = mock_batch_api
            mock_batch_api.read_namespaced_job.return_value = mock_job
            mock_batch_api.delete_namespaced_job.return_value = mock_delete_result

            import time

            original_time = time.time
            call_count = [0]

            def fake_time():
                call_count[0] += 1
                return original_time() + call_count[0] * 10000

            with patch("lean_runner.time.time", side_effect=fake_time):
                with pytest.raises(TimeoutError) as exc_info:
                    lean_runner.run_lean_backtest(
                        "abcdef12-1234-1234-1234-123456789abc",
                        tmpdir,
                        timeout_seconds=1,
                    )

            assert "timed out" in str(exc_info.value).lower()
            mock_batch_api.delete_namespaced_job.assert_called_once()


def test_is_container_running():
    with patch("lean_runner.k8s_client.CoreV1Api") as mock_core:
        mock_core_api = MagicMock()
        mock_core.return_value = mock_core_api

        # Running pod
        mock_pod = MagicMock()
        mock_pod.status.phase = "Running"
        mock_core_api.list_namespaced_pod.return_value.items = [mock_pod]
        assert lean_runner.is_container_running("lean-live-abcdef12") is True

        # No pods
        mock_core_api.list_namespaced_pod.return_value.items = []
        assert lean_runner.is_container_running("lean-live-abcdef12") is False

        # Pending pod (not Running)
        mock_pod2 = MagicMock()
        mock_pod2.status.phase = "Pending"
        mock_core_api.list_namespaced_pod.return_value.items = [mock_pod2]
        assert lean_runner.is_container_running("lean-live-abcdef12") is False


def test_upload_job_inputs():
    with tempfile.TemporaryDirectory() as tmpdir:
        # Create some test files
        os.makedirs(os.path.join(tmpdir, "algorithm"))
        with open(os.path.join(tmpdir, "algorithm", "main.py"), "w") as f:
            f.write("class MyStrategy: pass")
        with open(os.path.join(tmpdir, "config.json"), "w") as f:
            json.dump({"environment": "backtesting"}, f)

        mock_s3 = MagicMock()
        with patch("lean_runner._get_s3", return_value=mock_s3):
            lean_runner.upload_job_inputs("test-job-id", tmpdir)

        # Should have called upload_file for each file
        assert mock_s3.upload_file.call_count == 2
        # Verify the bucket arg (second positional arg) is always the configured bucket
        call_args = [c[0][1] for c in mock_s3.upload_file.call_args_list]
        assert all(a == lean_runner.S3_BUCKET for a in call_args)


def _make_paginator(pages: list) -> MagicMock:
    """Helper: build a mock paginator that yields the given pages."""
    mock_paginator = MagicMock()
    mock_paginator.paginate.return_value = iter(pages)
    return mock_paginator


def test_poll_live_results_none():
    """poll_live_results returns None when no objects exist in S3."""
    mock_s3 = MagicMock()
    mock_s3.get_paginator.return_value = _make_paginator([{"Contents": []}])

    with patch("lean_runner._get_s3", return_value=mock_s3):
        result = lean_runner.poll_live_results("test-job-id")

    assert result is None


def test_poll_live_results_no_json_files():
    """poll_live_results returns None when S3 objects exist but none are .json."""
    mock_s3 = MagicMock()
    mock_s3.get_paginator.return_value = _make_paginator(
        [{"Contents": [{"Key": "jobs/test-job-id/results/output.csv"}]}]
    )

    with patch("lean_runner._get_s3", return_value=mock_s3):
        result = lean_runner.poll_live_results("test-job-id")

    assert result is None


def test_poll_live_results_returns_dict():
    """poll_live_results parses and returns the latest JSON from S3."""
    import io as _io

    payload = {"equity": 10000}
    mock_s3 = MagicMock()
    mock_s3.get_paginator.return_value = _make_paginator(
        [{"Contents": [{"Key": "jobs/test-job-id/results/snapshot.json"}]}]
    )

    buf = _io.BytesIO(json.dumps(payload).encode())

    def fake_download_fileobj(bucket, key, fileobj):
        fileobj.write(buf.getvalue())

    mock_s3.download_fileobj.side_effect = fake_download_fileobj

    with patch("lean_runner._get_s3", return_value=mock_s3):
        result = lean_runner.poll_live_results("test-job-id")

    assert result == payload


def test_poll_live_results_returns_latest():
    """poll_live_results returns the alphabetically last .json key."""
    import io as _io

    payload = {"equity": 99999}
    mock_s3 = MagicMock()
    # Two pages, multiple .json keys; latest alphabetically is snapshot_z.json
    mock_s3.get_paginator.return_value = _make_paginator(
        [
            {
                "Contents": [
                    {"Key": "jobs/test-job-id/results/snapshot_a.json"},
                    {"Key": "jobs/test-job-id/results/snapshot_m.json"},
                ]
            },
            {
                "Contents": [
                    {"Key": "jobs/test-job-id/results/snapshot_z.json"},
                ]
            },
        ]
    )

    buf = _io.BytesIO(json.dumps(payload).encode())

    def fake_download_fileobj(bucket, key, fileobj):
        assert key == "jobs/test-job-id/results/snapshot_z.json"
        fileobj.write(buf.getvalue())

    mock_s3.download_fileobj.side_effect = fake_download_fileobj

    with patch("lean_runner._get_s3", return_value=mock_s3):
        result = lean_runner.poll_live_results("test-job-id")

    assert result == payload


def test_poll_live_results_invalid_json():
    """poll_live_results returns None when the JSON file is malformed."""
    import io as _io

    mock_s3 = MagicMock()
    mock_s3.get_paginator.return_value = _make_paginator(
        [{"Contents": [{"Key": "jobs/test-job-id/results/bad.json"}]}]
    )

    def fake_download_fileobj(bucket, key, fileobj):
        fileobj.write(b"not valid json {{")

    mock_s3.download_fileobj.side_effect = fake_download_fileobj

    with patch("lean_runner._get_s3", return_value=mock_s3):
        result = lean_runner.poll_live_results("test-job-id")

    assert result is None


def test_run_lean_backtest_job_failure():
    """RuntimeError is raised when the K8s job fails."""
    with tempfile.TemporaryDirectory() as tmpdir:
        mock_job = MagicMock()
        mock_job.status.succeeded = 0
        mock_job.status.failed = 1

        mock_pod = MagicMock()
        mock_pod.metadata.name = "lean-backtest-pod"

        with patch("lean_runner.k8s_client.BatchV1Api") as mock_batch, patch(
            "lean_runner.k8s_client.CoreV1Api"
        ) as mock_core, patch("lean_runner.upload_job_inputs"):

            mock_batch_api = MagicMock()
            mock_batch.return_value = mock_batch_api
            mock_batch_api.read_namespaced_job.return_value = mock_job

            mock_core_api = MagicMock()
            mock_core.return_value = mock_core_api
            mock_core_api.list_namespaced_pod.return_value.items = [mock_pod]
            mock_core_api.read_namespaced_pod_log.return_value = "LEAN fatal error"

            with pytest.raises(RuntimeError, match="LEAN job failed"):
                lean_runner.run_lean_backtest(
                    "abcdef12-1234-1234-1234-123456789abc", tmpdir, timeout_seconds=60
                )

            mock_batch_api.delete_namespaced_job.assert_called_once()


def test_live_pod_startup_timeout():
    """RuntimeError is raised and job deleted when pod never reaches Running within 120s."""
    with tempfile.TemporaryDirectory() as tmpdir:
        with patch("lean_runner.k8s_client.BatchV1Api") as mock_batch, patch(
            "lean_runner.k8s_client.CoreV1Api"
        ) as mock_core, patch("lean_runner.upload_job_inputs"), patch(
            "lean_runner.time.sleep"
        ):

            mock_batch_api = MagicMock()
            mock_batch.return_value = mock_batch_api

            mock_core_api = MagicMock()
            mock_core.return_value = mock_core_api
            # Pod stuck in Pending — items list is always empty
            mock_core_api.list_namespaced_pod.return_value.items = []

            # Make time.time() exhaust the deadline immediately:
            # first call (sets deadline) returns 0, second call (loop condition) returns 121
            call_count = [0]

            def fake_time():
                call_count[0] += 1
                return 0 if call_count[0] == 1 else 121

            with patch("lean_runner.time.time", side_effect=fake_time):
                with pytest.raises(RuntimeError, match="never reached Running"):
                    lean_runner.run_lean_live(
                        "abcdef12-1234-1234-1234-123456789abc", tmpdir
                    )

            mock_batch_api.delete_namespaced_job.assert_called_once()
