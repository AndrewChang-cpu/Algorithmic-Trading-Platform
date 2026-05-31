import glob
import io
import json
import logging
import os
import re
import time
from pathlib import Path
from typing import Optional

import boto3
from kubernetes import client as k8s_client, config as k8s_config

# Logging setup — all logs to /logs per project convention
_log_dir = (
    "/logs"
    if os.path.exists("/logs")
    else os.path.join(os.path.dirname(__file__), "..", "logs")
)
os.makedirs(_log_dir, exist_ok=True)
logging.basicConfig(
    filename=os.path.join(_log_dir, "lean_runner.log"),
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "service": "lean_runner", "level": "%(levelname)s", "message": "%(message)s"}',
)
logger = logging.getLogger(__name__)

LEAN_IMAGE = os.environ.get("LEAN_IMAGE", "lean-atp:latest")
NAMESPACE = os.environ.get("K8S_NAMESPACE", "default")
S3_BUCKET = os.environ["S3_BUCKET"]
S3_ACCESS_KEY = os.environ["S3_ACCESS_KEY"]
S3_SECRET_KEY = os.environ["S3_SECRET_KEY"]
S3_ENDPOINT = os.environ.get("S3_ENDPOINT")
S3_REGION = os.environ.get("S3_REGION", "us-east-1")

# Load K8s config — log warning if no config available (e.g. local dev without kubeconfig)
try:
    k8s_config.load_incluster_config()
except k8s_config.ConfigException:
    try:
        k8s_config.load_kube_config()
    except k8s_config.ConfigException:
        logger.warning(
            "No Kubernetes configuration found; K8s API calls will fail at runtime"
        )

_UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)


def _get_s3():
    kwargs = dict(
        region_name=S3_REGION,
        aws_access_key_id=S3_ACCESS_KEY,
        aws_secret_access_key=S3_SECRET_KEY,
    )
    if S3_ENDPOINT:
        kwargs["endpoint_url"] = S3_ENDPOINT
    return boto3.client("s3", **kwargs)


def upload_job_inputs(job_id: str, job_dir: str) -> None:
    """Upload all files from job_dir/ tree to s3://S3_BUCKET/jobs/{job_id}/input/."""
    s3 = _get_s3()
    job_dir_path = Path(job_dir)
    for local_file in job_dir_path.rglob("*"):
        if local_file.is_file():
            relative = local_file.relative_to(job_dir_path)
            s3_key = f"jobs/{job_id}/input/{relative}"
            logger.info(f"Uploading {local_file} to s3://{S3_BUCKET}/{s3_key}")
            s3.upload_file(str(local_file), S3_BUCKET, s3_key)


def download_job_results(job_id: str, dest_dir: str) -> None:
    """Download all objects with prefix jobs/{job_id}/results/ from S3 to dest_dir."""
    s3 = _get_s3()
    prefix = f"jobs/{job_id}/results/"
    paginator = s3.get_paginator("list_objects_v2")
    for page in paginator.paginate(Bucket=S3_BUCKET, Prefix=prefix):
        for obj in page.get("Contents", []):
            key = obj["Key"]
            relative = key[len(prefix) :]
            if not relative:
                continue
            local_path = os.path.join(dest_dir, relative)
            os.makedirs(os.path.dirname(local_path), exist_ok=True)
            logger.info(f"Downloading s3://{S3_BUCKET}/{key} to {local_path}")
            s3.download_file(S3_BUCKET, key, local_path)


def cleanup_job_s3(job_id: str) -> None:
    """Delete all objects under prefix jobs/{job_id}/ in S3."""
    s3 = _get_s3()
    prefix = f"jobs/{job_id}/"
    paginator = s3.get_paginator("list_objects_v2")
    for page in paginator.paginate(Bucket=S3_BUCKET, Prefix=prefix):
        contents = page.get("Contents", [])
        if not contents:
            continue
        # Batch delete up to 1000 keys per call
        for i in range(0, len(contents), 1000):
            batch = contents[i : i + 1000]
            objects = [{"Key": obj["Key"]} for obj in batch]
            s3.delete_objects(Bucket=S3_BUCKET, Delete={"Objects": objects})
            logger.info(f"Deleted {len(objects)} S3 objects under {prefix}")


def _build_lean_job_spec(job_name: str, job_id: str, job_type: str) -> dict:
    """Returns a Kubernetes Job spec dict."""
    env = [
        {"name": "JOB_ID", "value": job_id},
        {"name": "S3_BUCKET", "value": S3_BUCKET},
        {
            "name": "S3_ACCESS_KEY",
            "valueFrom": {
                "secretKeyRef": {"name": "atp-core-credentials", "key": "S3_ACCESS_KEY"}
            },
        },
        {
            "name": "S3_SECRET_KEY",
            "valueFrom": {
                "secretKeyRef": {"name": "atp-core-credentials", "key": "S3_SECRET_KEY"}
            },
        },
    ]
    if job_type == "live":
        env.append({"name": "KAFKA_BOOTSTRAP_SERVERS", "value": "kafka:9092"})
    if S3_ENDPOINT:
        env.append({"name": "S3_ENDPOINT", "value": S3_ENDPOINT})

    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": job_name,
            "labels": {
                "app": "lean-runner",
                "job_id": job_id,
            },
        },
        "spec": {
            "backoffLimit": 0,
            "ttlSecondsAfterFinished": 3600,
            "template": {
                "spec": {
                    "restartPolicy": "Never",
                    "terminationGracePeriodSeconds": 120,
                    "nodeSelector": {"dedicated": "lean-worker"},
                    "tolerations": [
                        {
                            "key": "dedicated",
                            "operator": "Equal",
                            "value": "lean-worker",
                            "effect": "NoSchedule",
                        }
                    ],
                    "containers": [
                        {
                            "name": "lean",
                            "image": LEAN_IMAGE,
                            "env": env,
                            "resources": {
                                "requests": {"memory": "1Gi", "cpu": "1"},
                                "limits": {"memory": "3Gi", "cpu": "2"},
                            },
                            "securityContext": {
                                "runAsNonRoot": True,
                                "allowPrivilegeEscalation": False,
                                "capabilities": {"drop": ["ALL"]},
                            },
                        }
                    ],
                }
            },
        },
    }


def run_lean_backtest(job_id: str, job_dir: str, timeout_seconds: int = 7200) -> str:
    """
    Run LEAN in backtest mode via a Kubernetes Job. Blocks until completion or timeout.

    Returns path to results JSON file.
    Raises TimeoutError if job exceeds timeout_seconds.
    Raises RuntimeError if job fails.
    """
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    logger.info(f"Starting backtest K8s job for job {job_id}")

    upload_job_inputs(job_id, job_dir)

    job_name = f"lean-backtest-{job_id[:8]}-{int(time.time()) % 10000}"
    spec_dict = _build_lean_job_spec(job_name, job_id, "backtest")

    batch_api = k8s_client.BatchV1Api()
    batch_api.create_namespaced_job(NAMESPACE, body=spec_dict)
    logger.info(f"Created K8s job {job_name}")

    deadline = time.time() + timeout_seconds
    while time.time() < deadline:
        job = batch_api.read_namespaced_job(job_name, NAMESPACE)
        if job.status.succeeded:
            logger.info(f"K8s job {job_name} succeeded")
            break
        if job.status.failed:
            pods = k8s_client.CoreV1Api().list_namespaced_pod(
                NAMESPACE, label_selector=f"job-name={job_name}"
            )
            logs = ""
            if pods.items:
                try:
                    logs = k8s_client.CoreV1Api().read_namespaced_pod_log(
                        pods.items[0].metadata.name, NAMESPACE, tail_lines=50
                    )
                except Exception:
                    pass
            batch_api.delete_namespaced_job(
                job_name, NAMESPACE, propagation_policy="Foreground"
            )
            raise RuntimeError(f"LEAN job failed. Logs: {logs[:500]}")
        time.sleep(10)
    else:
        batch_api.delete_namespaced_job(
            job_name, NAMESPACE, propagation_policy="Foreground"
        )
        raise TimeoutError(f"LEAN job timed out after {timeout_seconds}s")

    local_results_dir = os.path.join("/tmp/atp-jobs", job_id, "results")
    os.makedirs(local_results_dir, exist_ok=True)
    download_job_results(job_id, local_results_dir)

    matches = glob.glob(os.path.join(local_results_dir, "*.json"))
    if not matches:
        raise RuntimeError("No results JSON found")
    # Prefer summary file if present
    summary = [m for m in matches if m.endswith("-summary.json")]
    return summary[0] if summary else matches[0]


def run_lean_live(job_id: str, job_dir: str) -> str:
    """
    Start LEAN in live trading mode via a Kubernetes Job.

    Returns job_name string once the pod enters Running phase (max 120s).
    """
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    logger.info(f"Starting live K8s job for job {job_id}")

    upload_job_inputs(job_id, job_dir)

    job_name = f"lean-live-{job_id[:8]}"
    spec_dict = _build_lean_job_spec(job_name, job_id, "live")

    batch_api = k8s_client.BatchV1Api()
    batch_api.create_namespaced_job(NAMESPACE, body=spec_dict)
    logger.info(f"Created K8s live job {job_name}")

    deadline = time.time() + 120
    while time.time() < deadline:
        pods = k8s_client.CoreV1Api().list_namespaced_pod(
            NAMESPACE, label_selector=f"job-name={job_name}"
        )
        if pods.items and pods.items[0].status.phase == "Running":
            logger.info(f"Live job {job_name} pod is Running")
            break
        time.sleep(5)
    else:
        batch_api.delete_namespaced_job(
            job_name,
            NAMESPACE,
            body=k8s_client.V1DeleteOptions(propagation_policy="Foreground"),
        )
        raise RuntimeError(
            f"LEAN live pod for job {job_id} never reached Running within 120s"
        )
    return job_name


def stop_lean_live(job_name: str, job_dir: str) -> Optional[str]:
    """
    Stop a running live LEAN K8s Job gracefully.

    Returns path to results JSON if available, else None.
    """
    logger.info(f"Stopping live K8s job {job_name}")

    # Retrieve the full job_id from the Job's labels
    job_id = ""
    try:
        job = k8s_client.BatchV1Api().read_namespaced_job(job_name, NAMESPACE)
        job_id = (job.metadata.labels or {}).get("job_id", "")
    except Exception as e:
        logger.warning(f"Could not read job labels for {job_name}: {e}")

    k8s_client.BatchV1Api().delete_namespaced_job(
        job_name, NAMESPACE, propagation_policy="Foreground"
    )

    # Wait for pod deletion (max 60s)
    deadline = time.time() + 60
    while time.time() < deadline:
        pods = k8s_client.CoreV1Api().list_namespaced_pod(
            NAMESPACE, label_selector=f"job-name={job_name}"
        )
        if not pods.items:
            break
        time.sleep(5)

    if not job_id:
        return None

    local_results_dir = os.path.join("/tmp/atp-jobs", job_id, "results")
    os.makedirs(local_results_dir, exist_ok=True)
    download_job_results(job_id, local_results_dir)

    matches = glob.glob(os.path.join(local_results_dir, "*.json"))
    summary = [m for m in matches if m.endswith("-summary.json")]
    if summary:
        return summary[0]
    return matches[0] if matches else None


def is_container_running(job_name: str) -> bool:
    """Return True if any pod for the given K8s job is in Running phase."""
    pods = k8s_client.CoreV1Api().list_namespaced_pod(
        NAMESPACE, label_selector=f"job-name={job_name}"
    )
    return any(p.status.phase == "Running" for p in pods.items)


def poll_live_results(job_id: str) -> Optional[dict]:
    """
    Check if results prefix exists in S3; download and parse the latest JSON or return None.
    """
    s3 = _get_s3()
    prefix = f"jobs/{job_id}/results/"
    paginator = s3.get_paginator("list_objects_v2")
    json_keys = []
    for page in paginator.paginate(Bucket=S3_BUCKET, Prefix=prefix):
        json_keys.extend(
            obj["Key"]
            for obj in page.get("Contents", [])
            if obj["Key"].endswith(".json")
        )
    if not json_keys:
        return None
    latest_key = sorted(json_keys)[-1]
    buf = io.BytesIO()
    s3.download_fileobj(S3_BUCKET, latest_key, buf)
    buf.seek(0)
    try:
        return json.loads(buf.read())
    except Exception:
        return None
