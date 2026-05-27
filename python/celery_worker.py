import json
import logging
import os
import shutil
import tempfile
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path

import boto3
import psycopg2
import psycopg2.extras
import requests
from celery import Celery
from confluent_kafka import Producer

from data_materializer import materialize_lean_csv
from lean_runner import (is_container_running, poll_live_results,
                          run_lean_backtest, run_lean_live, stop_lean_live)
from results_parser import is_runtime_error, parse_equity_curve, parse_performance_metrics
from strategy_validator import validate_strategy

# Configure logging
log_dir = "/logs" if os.path.exists("/logs") else os.path.join(os.path.dirname(__file__), "..", "logs")
os.makedirs(log_dir, exist_ok=True)
logging.basicConfig(
    filename=os.path.join(log_dir, "celery.log"),
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "service": "celery_worker", "level": "%(levelname)s", "job_id": "%(job_id)s", "message": "%(message)s"}'
)

REDIS_URL = os.environ.get("REDIS_URL", "redis://localhost:6379/0")
DATABASE_URL = os.environ.get("DATABASE_URL", "postgres://postgres:password@localhost:5432/atp")
GO_DATA_URL = os.environ.get("GO_DATA_URL", "http://localhost:8081")
LEAN_JOB_TMP_DIR = os.environ.get("LEAN_JOB_TMP_DIR", "/tmp/atp-jobs")
S3_ENDPOINT = os.environ.get("S3_ENDPOINT")
S3_ACCESS_KEY = os.environ.get("S3_ACCESS_KEY", "minioadmin")
S3_SECRET_KEY = os.environ.get("S3_SECRET_KEY", "minioadmin")
S3_BUCKET = os.environ.get("S3_BUCKET", "atp-strategies")
S3_REGION = os.environ.get("S3_REGION", "us-east-1")
KAFKA_BOOTSTRAP_SERVERS = os.environ.get("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")
LEAN_KAFKA_BOOTSTRAP_SERVERS = os.environ.get("LEAN_KAFKA_BOOTSTRAP_SERVERS", "host.docker.internal:9092")

app = Celery("atp", broker=REDIS_URL, backend=REDIS_URL)

# ── helpers ──────────────────────────────────────────────────────────────────

def _get_db():
    return psycopg2.connect(DATABASE_URL, cursor_factory=psycopg2.extras.RealDictCursor)

def _get_s3():
    kwargs = dict(
        region_name=S3_REGION,
        aws_access_key_id=S3_ACCESS_KEY,
        aws_secret_access_key=S3_SECRET_KEY,
    )
    if S3_ENDPOINT:
        kwargs["endpoint_url"] = S3_ENDPOINT
    return boto3.client("s3", **kwargs)

def _get_redis():
    import redis
    return redis.from_url(REDIS_URL)

def _logger(job_id):
    return logging.LoggerAdapter(logging.getLogger(__name__), {"job_id": job_id})

def _update_job_status(conn, job_id, status, error_message=None):
    with conn.cursor() as cur:
        if status == "running":
            cur.execute(
                "UPDATE jobs SET status=%s, started_at=NOW() WHERE id=%s",
                (status, job_id)
            )
        elif status in ("completed", "failed"):
            cur.execute(
                "UPDATE jobs SET status=%s, completed_at=NOW(), error_message=%s WHERE id=%s",
                (status, error_message, job_id)
            )
        else:
            cur.execute("UPDATE jobs SET status=%s WHERE id=%s", (status, job_id))
    conn.commit()

def _fetch_job(conn, job_id):
    with conn.cursor() as cur:
        cur.execute("""
            SELECT j.*, sv.s3_key, sv.version_number
            FROM jobs j
            JOIN strategy_versions sv ON j.strategy_version_id = sv.id
            WHERE j.id = %s
        """, (job_id,))
        return dict(cur.fetchone())

def _download_strategy(s3, s3_key, dest_path):
    os.makedirs(os.path.dirname(dest_path), exist_ok=True)
    s3.download_file(S3_BUCKET, s3_key, dest_path)

def _fetch_market_data(conn, symbols, start_date, end_date, resolution):
    """Query market_data for materialized bars."""
    rows_by_symbol = {}
    with conn.cursor() as cur:
        for symbol in symbols:
            cur.execute("""
                SELECT time, open, high, low, close, volume
                FROM market_data
                WHERE symbol = %s AND resolution = %s
                  AND time >= %s AND time < %s
                ORDER BY time ASC
            """, (symbol, resolution, start_date, end_date))
            rows_by_symbol[symbol] = [dict(r) for r in cur.fetchall()]
    return rows_by_symbol

def _write_lean_backtest_config(job_dir, class_name):
    config = {
        "environment": "backtesting",
        "algorithm-type-name": class_name,
        "algorithm-language": "Python",
        "algorithm-location": "/lean/algorithm/main.py",
        "data-folder": "/lean/data",
        "results-destination-folder": "/lean/results",
        "environments": {
            "backtesting": {
                "live-mode": False,
                "setup-handler": "QuantConnect.Lean.Engine.Setup.BacktestingSetupHandler",
                "result-handler": "QuantConnect.Lean.Engine.Results.BacktestingResultHandler",
                "data-feed-handler": "QuantConnect.Lean.Engine.DataFeeds.FileSystemDataFeed",
                "real-time-handler": "QuantConnect.Lean.Engine.RealTime.BacktestingRealTimeHandler",
                "history-provider": "QuantConnect.Lean.Engine.HistoricalData.SubscriptionDataReaderHistoryProvider",
                "transaction-handler": "QuantConnect.Lean.Engine.TransactionHandlers.BacktestingTransactionHandler",
            }
        }
    }
    os.makedirs(job_dir, exist_ok=True)
    with open(os.path.join(job_dir, "config.json"), "w") as f:
        json.dump(config, f, indent=2)

def _write_lean_live_config(job_dir, class_name, job_id):
    config = {
        "environment": "live-paper",
        "algorithm-type-name": class_name,
        "algorithm-language": "Python",
        "algorithm-location": "/lean/algorithm/main.py",
        "data-folder": "/lean/data",
        "results-destination-folder": "/lean/results",
        "job-id": job_id,
        "kafka-bootstrap-servers": LEAN_KAFKA_BOOTSTRAP_SERVERS,
        "environments": {
            "live-paper": {
                "live-mode": True,
                "live-mode-brokerage": "PaperBrokerage",
                "setup-handler": "QuantConnect.Lean.Engine.Setup.BrokerageSetupHandler",
                "result-handler": "QuantConnect.Lean.Engine.Results.LiveTradingResultHandler",
                "data-feed-handler": "QuantConnect.Lean.Engine.DataFeeds.LiveTradingDataFeed",
                "data-queue-handler": ["KafkaDataQueueHandler"],
                "real-time-handler": "QuantConnect.Lean.Engine.RealTime.LiveTradingRealTimeHandler",
                "transaction-handler": "QuantConnect.Lean.Engine.TransactionHandlers.BacktestingTransactionHandler",
                "history-provider": ["QuantConnect.Lean.Engine.HistoricalData.SubscriptionDataReaderHistoryProvider"],
            }
        }
    }
    os.makedirs(job_dir, exist_ok=True)
    with open(os.path.join(job_dir, "config.json"), "w") as f:
        json.dump(config, f, indent=2)

def _store_results(conn, job_id, results_json):
    metrics = parse_performance_metrics(results_json)
    metrics["job_id"] = job_id
    cols = ", ".join(metrics.keys())
    placeholders = ", ".join(["%s"] * len(metrics))
    with conn.cursor() as cur:
        cur.execute(
            f"INSERT INTO performance_metrics ({cols}) VALUES ({placeholders}) ON CONFLICT (job_id) DO NOTHING",
            list(metrics.values())
        )

    points = parse_equity_curve(results_json)
    if points:
        with conn.cursor() as cur:
            psycopg2.extras.execute_batch(cur, """
                INSERT INTO portfolio_metrics (time, job_id, open, high, low, close)
                VALUES (%s, %s, %s, %s, %s, %s)
                ON CONFLICT DO NOTHING
            """, [(p["time"], job_id, p["open"], p["high"], p["low"], p["close"]) for p in points])
    conn.commit()

def _publish_portfolio_snapshot(producer, job_id, snapshot):
    msg = json.dumps({"job_id": job_id, **snapshot}).encode()
    producer.produce("portfolio_data", key=job_id, value=msg)
    producer.poll(0)


# ── tasks ─────────────────────────────────────────────────────────────────────

@app.task(name="atp.run_lean_backtest", bind=True)
def run_lean_backtest_task(self, job_id: str):
    log = _logger(job_id)
    log.info("Starting backtest task")
    conn = _get_db()
    job_dir = os.path.join(LEAN_JOB_TMP_DIR, job_id)

    try:
        job = _fetch_job(conn, job_id)
        symbols = job["symbols"]
        resolution = job["resolution"]
        start_date = str(job["start_date"])
        end_date = str(job["end_date"])
        timeout = job.get("timeout_seconds", 7200)

        # Download strategy
        algo_path = os.path.join(job_dir, "algorithm", "main.py")
        _download_strategy(_get_s3(), job["s3_key"], algo_path)

        # Double-check AST safety
        with open(algo_path) as f:
            source = f.read()
        validation = validate_strategy(source)
        if not validation["valid"]:
            raise ValueError(f"Strategy validation failed: {validation['violation']}")
        class_name = validation["class_name"]

        # Ensure market data is cached
        resp = requests.post(
            f"{GO_DATA_URL}/data/historical",
            json={"symbols": symbols, "start_date": start_date, "end_date": end_date, "resolution": resolution},
            timeout=300
        )
        resp.raise_for_status()

        # Materialize CSV files
        rows_by_symbol = _fetch_market_data(conn, symbols, start_date, end_date, resolution)
        data_dir = os.path.join(job_dir, "data")
        for symbol, rows in rows_by_symbol.items():
            materialize_lean_csv(rows, data_dir, symbol, resolution)

        # Write config and run
        _write_lean_backtest_config(job_dir, class_name)
        _update_job_status(conn, job_id, "running")

        results_path = run_lean_backtest(job_id, job_dir, timeout)

        with open(results_path) as f:
            results_json = json.load(f)

        err, msg = is_runtime_error(results_json)
        if err:
            raise RuntimeError(f"LEAN runtime error: {msg}")

        _store_results(conn, job_id, results_json)
        _update_job_status(conn, job_id, "completed")
        log.info("Backtest completed successfully")

    except TimeoutError as e:
        _update_job_status(conn, job_id, "failed", str(e))
        log.error(f"Timeout: {e}")
    except Exception as e:
        _update_job_status(conn, job_id, "failed", str(e))
        log.error(f"Failed: {e}")
    finally:
        conn.close()
        if os.path.exists(job_dir):
            shutil.rmtree(job_dir, ignore_errors=True)


@app.task(name="atp.run_lean_live", bind=True)
def run_lean_live_task(self, job_id: str):
    log = _logger(job_id)
    log.info("Starting live task")
    conn = _get_db()
    r = _get_redis()
    job_dir = os.path.join(LEAN_JOB_TMP_DIR, job_id)
    container_id = None

    try:
        job = _fetch_job(conn, job_id)
        symbols = job["symbols"]
        resolution = job["resolution"]
        warmup_days = job.get("warmup_days") or 365
        today = datetime.now(tz=timezone.utc).date()
        warmup_start = (today - timedelta(days=warmup_days)).isoformat()
        today_str = today.isoformat()

        # Download + validate strategy
        algo_path = os.path.join(job_dir, "algorithm", "main.py")
        _download_strategy(_get_s3(), job["s3_key"], algo_path)
        with open(algo_path) as f:
            source = f.read()
        validation = validate_strategy(source)
        if not validation["valid"]:
            raise ValueError(f"Strategy validation failed: {validation['violation']}")
        class_name = validation["class_name"]

        # Warmup data
        resp = requests.post(
            f"{GO_DATA_URL}/data/historical",
            json={"symbols": symbols, "start_date": warmup_start, "end_date": today_str, "resolution": resolution},
            timeout=300
        )
        resp.raise_for_status()

        rows_by_symbol = _fetch_market_data(conn, symbols, warmup_start, today_str, resolution)
        data_dir = os.path.join(job_dir, "data")
        for symbol, rows in rows_by_symbol.items():
            materialize_lean_csv(rows, data_dir, symbol, resolution)

        _write_lean_live_config(job_dir, class_name, job_id)
        _update_job_status(conn, job_id, "running")

        container_id = run_lean_live(job_id, job_dir)
        r.set(f"job:{job_id}:container", container_id, ex=86400)

        producer = Producer({"bootstrap.servers": KAFKA_BOOTSTRAP_SERVERS})

        # Polling loop
        while True:
            time.sleep(5)

            # Check stop signal
            if r.get(f"job:{job_id}:stop"):
                log.info("Stop signal received")
                break

            # Check container health
            if not is_container_running(container_id):
                log.info("Container exited on its own")
                break

            # Poll and publish results snapshot
            snapshot = poll_live_results(job_dir)
            if snapshot:
                runtime = snapshot.get("runtimeStatistics", {})
                _publish_portfolio_snapshot(producer, job_id, {
                    "equity": runtime.get("Equity", "0").replace("$", "").replace(",", ""),
                    "unrealized": runtime.get("Unrealized", "0").replace("$", "").replace(",", ""),
                    "holdings": runtime.get("Holdings", "0").replace("$", "").replace(",", ""),
                    "fees": runtime.get("Fees", "0").replace("-$", "").replace("$", "").replace(",", ""),
                    "time": datetime.now(tz=timezone.utc).isoformat(),
                })

        producer.flush()

        # Final results
        final_path = stop_lean_live(container_id, job_dir)
        if final_path:
            with open(final_path) as f:
                results_json = json.load(f)
            _store_results(conn, job_id, results_json)

        _update_job_status(conn, job_id, "completed")
        log.info("Live job completed")

    except Exception as e:
        _update_job_status(conn, job_id, "failed", str(e))
        log.error(f"Live job failed: {e}")
        if container_id:
            stop_lean_live(container_id, job_dir)
    finally:
        conn.close()
        if os.path.exists(job_dir):
            shutil.rmtree(job_dir, ignore_errors=True)
