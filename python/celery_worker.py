import json
import logging
import os
import re
import shutil
import subprocess
import time
from datetime import datetime, timedelta, timezone

import boto3
import psycopg2
import psycopg2.extras
import requests
from celery import Celery
from celery.signals import worker_init
from confluent_kafka import Producer

from data_materializer import materialize_lean_csv
from lean_runner import (
    is_container_running,
    poll_live_results,
    run_lean_backtest,
    run_lean_live,
    stop_lean_live,
)
from results_parser import (
    is_runtime_error,
    parse_equity_curve,
    parse_performance_metrics,
)
from strategy_validator import validate_strategy


@worker_init.connect
def _create_lean_network(**kwargs):
    if not os.path.exists("/var/run/docker.sock"):
        return
    try:
        result = subprocess.run(
            ["docker", "network", "create", "lean-live-net", "--driver", "bridge"],
            capture_output=True,
            timeout=10,
        )
        if result.returncode != 0 and b"already exists" not in result.stderr:
            logging.getLogger(__name__).warning(
                "docker network create failed: %s", result.stderr.decode()
            )
    except subprocess.TimeoutExpired:
        logging.getLogger(__name__).warning("docker network create timed out")


# Configure logging
log_dir = (
    "/logs"
    if os.path.exists("/logs")
    else os.path.join(os.path.dirname(__file__), "..", "logs")
)
os.makedirs(log_dir, exist_ok=True)
logging.basicConfig(
    filename=os.path.join(log_dir, "celery.log"),
    level=logging.INFO,
    format='{"timestamp": "%(asctime)s", "service": "celery_worker", "level": "%(levelname)s", "job_id": "%(job_id)s", "message": "%(message)s"}',
)


class _DefaultJobIDFilter(logging.Filter):
    def filter(self, record):
        if not hasattr(record, "job_id"):
            record.job_id = "-"
        return True


logging.getLogger().addFilter(_DefaultJobIDFilter())


def _require_env(name: str) -> str:
    val = os.environ.get(name)
    if not val:
        raise EnvironmentError(f"Required environment variable '{name}' is not set")
    return val


REDIS_URL = os.environ.get("REDIS_URL", "redis://localhost:6379/0")
DATABASE_URL = _require_env("DATABASE_URL")
GO_DATA_URL = os.environ.get("GO_DATA_URL", "http://localhost:8081")
LEAN_JOB_TMP_DIR = os.environ.get("LEAN_JOB_TMP_DIR", "/tmp/atp-jobs")
S3_ENDPOINT = os.environ.get("S3_ENDPOINT")
S3_ACCESS_KEY = _require_env("S3_ACCESS_KEY")
S3_SECRET_KEY = _require_env("S3_SECRET_KEY")
S3_BUCKET = _require_env("S3_BUCKET")
S3_REGION = os.environ.get("S3_REGION", "us-east-1")
KAFKA_BOOTSTRAP_SERVERS = os.environ.get("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092")

app = Celery("atp", broker=REDIS_URL, backend=REDIS_URL)

_UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

PERFORMANCE_METRICS_COLS = (
    "job_id",
    "total_return_pct",
    "benchmark_return_pct",
    "compounding_annual_return",
    "alpha",
    "beta",
    "sharpe_ratio",
    "sortino_ratio",
    "max_drawdown_pct",
    "max_drawdown_duration_days",
    "drawdown_recovery_days",
    "volatility_annual",
    "annual_variance",
    "information_ratio",
    "tracking_error",
    "treynor_ratio",
    "probabilistic_sharpe_ratio",
    "value_at_risk_99",
    "value_at_risk_95",
    "total_trades",
    "winning_trades",
    "losing_trades",
    "win_rate_pct",
    "loss_rate_pct",
    "avg_win_pct",
    "avg_loss_pct",
    "profit_loss_ratio",
    "expectancy",
    "total_fees",
    "avg_trade_duration",
    "max_consecutive_wins",
    "max_consecutive_losses",
    "largest_win",
    "largest_loss",
    "avg_mae",
    "avg_mfe",
    "start_equity",
    "end_equity",
    "net_profit",
    "portfolio_turnover",
    "estimated_capacity",
)

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


def _strip_currency(v) -> float:
    s = str(v)
    stripped = s.replace("$", "").replace(",", "").strip()
    negative = stripped.startswith("-")
    cleaned = stripped.lstrip("+-")
    try:
        result = float(cleaned) if cleaned else 0.0
    except ValueError:
        result = 0.0
    return -result if negative else result


def _logger(job_id):
    return logging.LoggerAdapter(logging.getLogger(__name__), {"job_id": job_id})


def _sanitize_error(msg: str) -> str:
    msg = re.sub(r'/tmp/atp-jobs/[^\s"]+', "<job_dir>", msg)
    msg = re.sub(r"/[a-zA-Z0-9_/.-]+\.py", "<path>", msg)
    return msg[:300]



def _update_job_status(conn, job_id, status, error_message=None):
    with conn.cursor() as cur:
        if status == "running":
            cur.execute(
                "UPDATE jobs SET status=%s, started_at=NOW() WHERE id=%s",
                (status, job_id),
            )
        elif status in ("completed", "failed"):
            cur.execute(
                "UPDATE jobs SET status=%s, completed_at=NOW(), error_message=%s WHERE id=%s",
                (status, error_message, job_id),
            )
        else:
            cur.execute("UPDATE jobs SET status=%s WHERE id=%s", (status, job_id))
    conn.commit()


def _fetch_job(conn, job_id):
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT j.*, sv.s3_key, sv.version_number
            FROM jobs j
            JOIN strategy_versions sv ON j.strategy_version_id = sv.id
            WHERE j.id = %s
        """,
            (job_id,),
        )
        return dict(cur.fetchone())


def _download_strategy(s3, s3_key, dest_path):
    os.makedirs(os.path.dirname(dest_path), exist_ok=True)
    s3.download_file(S3_BUCKET, s3_key, dest_path)


def _fetch_market_data(conn, symbols, start_date, end_date, resolution):
    """Query market_data for materialized bars."""
    rows_by_symbol = {}
    with conn.cursor() as cur:
        for symbol in symbols:
            cur.execute(
                """
                SELECT time, open, high, low, close, volume
                FROM market_data
                WHERE symbol = %s AND resolution = %s
                  AND time >= %s AND time < %s
                ORDER BY time ASC
            """,
                (symbol, resolution, start_date, end_date),
            )
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
        },
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
        "kafka-bootstrap-servers": os.environ.get(
            "KAFKA_BOOTSTRAP_SERVERS", "kafka:9092"
        ),
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
                "history-provider": [
                    "QuantConnect.Lean.Engine.HistoricalData.SubscriptionDataReaderHistoryProvider"
                ],
            }
        },
    }
    os.makedirs(job_dir, exist_ok=True)
    with open(os.path.join(job_dir, "config.json"), "w") as f:
        json.dump(config, f, indent=2)


def _store_results(conn, job_id, results_json):
    metrics = parse_performance_metrics(results_json)
    metrics["job_id"] = job_id
    col_str = ", ".join(PERFORMANCE_METRICS_COLS)
    placeholders = ", ".join(f"%({col})s" for col in PERFORMANCE_METRICS_COLS)
    values = {col: metrics.get(col) for col in PERFORMANCE_METRICS_COLS}
    with conn.cursor() as cur:
        cur.execute(
            f"INSERT INTO performance_metrics ({col_str}) VALUES ({placeholders}) ON CONFLICT (job_id) DO NOTHING",
            values,
        )

    points = parse_equity_curve(results_json)
    if points:
        with conn.cursor() as cur:
            psycopg2.extras.execute_batch(
                cur,
                """
                INSERT INTO portfolio_metrics (time, job_id, open, high, low, close)
                VALUES (%s, %s, %s, %s, %s, %s)
                ON CONFLICT DO NOTHING
            """,
                [
                    (p["time"], job_id, p["open"], p["high"], p["low"], p["close"])
                    for p in points
                ],
            )
    conn.commit()


def _publish_portfolio_snapshot(producer, job_id, snapshot):
    msg = json.dumps({"job_id": job_id, **snapshot}).encode()
    producer.produce("portfolio_data", key=job_id, value=msg)
    producer.poll(0)


# ── tasks ─────────────────────────────────────────────────────────────────────


@app.task(name="atp.run_lean_backtest", bind=True)
def run_lean_backtest_task(self, job_id: str):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    log = _logger(job_id)
    log.info("Starting backtest task")
    conn = _get_db()
    job_dir = os.path.join(LEAN_JOB_TMP_DIR, job_id)

    try:
        job = _fetch_job(conn, job_id)
        if job["status"] != "queued":
            log.info(
                "Job %s is in status %s, skipping (cancelled before pickup)",
                job_id,
                job["status"],
            )
            return
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
            json={
                "symbols": symbols,
                "start_date": start_date,
                "end_date": end_date,
                "resolution": resolution,
            },
            timeout=300,
        )
        resp.raise_for_status()

        # Materialize CSV files
        rows_by_symbol = _fetch_market_data(
            conn, symbols, start_date, end_date, resolution
        )
        for symbol in symbols:
            if not rows_by_symbol.get(symbol):
                raise ValueError(
                    f"No market data available for {symbol} in requested range"
                )
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
        _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
        log.error(f"Timeout: {e}")
    except Exception as e:
        _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
        log.error(f"Failed: {e}")
    finally:
        conn.close()
        if os.path.exists(job_dir):
            shutil.rmtree(job_dir, ignore_errors=True)


@app.task(name="atp.run_lean_live", bind=True)
def run_lean_live_task(self, job_id: str):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    log = _logger(job_id)
    log.info("Starting live task")
    conn = _get_db()
    job_dir = os.path.join(LEAN_JOB_TMP_DIR, job_id)
    job_name = None
    producer = None
    _attempted_stop = False

    try:
        job = _fetch_job(conn, job_id)
        if job["status"] != "queued":
            log.info(
                "Job %s is in status %s, skipping (cancelled before pickup)",
                job_id,
                job["status"],
            )
            return
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
            json={
                "symbols": symbols,
                "start_date": warmup_start,
                "end_date": today_str,
                "resolution": resolution,
            },
            timeout=300,
        )
        resp.raise_for_status()

        rows_by_symbol = _fetch_market_data(
            conn, symbols, warmup_start, today_str, resolution
        )
        for symbol in symbols:
            if not rows_by_symbol.get(symbol):
                raise ValueError(
                    f"No market data available for {symbol} in requested range"
                )
        data_dir = os.path.join(job_dir, "data")
        for symbol, rows in rows_by_symbol.items():
            materialize_lean_csv(rows, data_dir, symbol, resolution)

        _write_lean_live_config(job_dir, class_name, job_id)
        _update_job_status(conn, job_id, "running")

        job_name = run_lean_live(job_id, job_dir)

        r = _get_redis()
        try:
            r.set(f"job:{job_id}:container", job_name, ex=86400)
            producer = Producer({"bootstrap.servers": KAFKA_BOOTSTRAP_SERVERS})

            while True:
                time.sleep(5)
                if r.get(f"job:{job_id}:stop"):
                    log.info("Stop signal received")
                    break
                if not is_container_running(job_name):
                    log.info("Container exited on its own")
                    break
                snapshot = poll_live_results(job_id)
                if snapshot:
                    runtime = snapshot.get("runtimeStatistics", {})
                    _publish_portfolio_snapshot(
                        producer,
                        job_id,
                        {
                            "equity": _strip_currency(runtime.get("Equity", "0")),
                            "unrealized": _strip_currency(
                                runtime.get("Unrealized", "0")
                            ),
                            "holdings": _strip_currency(runtime.get("Holdings", "0")),
                            "fees": _strip_currency(runtime.get("Fees", "0")),
                            "time": datetime.now(tz=timezone.utc).isoformat(),
                        },
                    )
        finally:
            r.close()

        _attempted_stop = True
        final_path = stop_lean_live(job_name, job_dir)
        if final_path:
            with open(final_path) as f:
                results_json = json.load(f)
            _store_results(conn, job_id, results_json)

        _update_job_status(conn, job_id, "completed")
        log.info("Live job completed")

    except Exception as e:
        _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
        log.error(f"Live job failed: {e}")
        if job_name and not _attempted_stop:
            try:
                stop_lean_live(job_name, job_dir)
            except Exception as stop_err:
                log.error("Failed to stop job %s: %s", job_name, stop_err)
    finally:
        if producer is not None:
            producer.flush(timeout=10)
        conn.close()
        if os.path.exists(job_dir):
            shutil.rmtree(job_dir, ignore_errors=True)
