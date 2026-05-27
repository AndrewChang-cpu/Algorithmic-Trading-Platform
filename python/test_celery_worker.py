import json
import os
import uuid
from unittest.mock import MagicMock, patch

import boto3
import psycopg2
import psycopg2.extras
import pytest
from moto import mock_aws
from testcontainers.postgres import PostgresContainer

_MIGRATIONS_DIR = os.path.join(os.path.dirname(__file__), "..", "migrations")

VALID_STRATEGY = """
class MyStrategy(QCAlgorithm):
    def Initialize(self):
        self.AddEquity("SPY", Resolution.Daily)
"""

INVALID_STRATEGY = "import os\nclass Bad(QCAlgorithm): pass\n"

LEAN_RESULTS = {
    "state": {"Status": "Completed"},
    "totalPerformance": {
        "portfolioStatistics": {
            "sharpeRatio": 1.5,
            "totalNetProfit": 0.15,
            "drawdown": 0.05,
            "compoundingAnnualReturn": 0.12,
        },
        "tradeStatistics": {}
    },
    "charts": {
        "Strategy Equity": {
            "series": {
                "Equity": {
                    "values": [
                        [1704153600, 100000, 101000, 99000, 100500],
                        [1704240000, 100500, 102000, 100000, 101500],
                        [1704326400, 101500, 103000, 101000, 102500],
                    ]
                }
            }
        }
    }
}

# portfolio_metrics without create_hypertable (plain Postgres)
_PORTFOLIO_DDL = """
CREATE TABLE portfolio_metrics (
  time TIMESTAMPTZ NOT NULL,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  open DECIMAL(20,4),
  high DECIMAL(20,4),
  low DECIMAL(20,4),
  close DECIMAL(20,4),
  PRIMARY KEY (job_id, time)
);
"""

# market_data without create_hypertable
_MARKET_DATA_DDL = """
CREATE TABLE IF NOT EXISTS market_data (
  time TIMESTAMPTZ NOT NULL,
  symbol VARCHAR(20) NOT NULL,
  resolution VARCHAR(10) NOT NULL,
  open DECIMAL(20,4),
  high DECIMAL(20,4),
  low DECIMAL(20,4),
  close DECIMAL(20,4),
  volume BIGINT
);
CREATE UNIQUE INDEX IF NOT EXISTS market_data_uniq ON market_data (symbol, resolution, time DESC);
"""


@pytest.fixture(scope="module")
def pg_dsn():
    with PostgresContainer("postgres:16-alpine") as container:
        raw = container.get_connection_url()
        dsn = raw.replace("postgresql+psycopg2://", "postgresql://")

        conn = psycopg2.connect(dsn)
        conn.autocommit = True
        cur = conn.cursor()

        for migration in [
            "001_create_users.up.sql",
            "002_create_strategies.up.sql",
            "003_create_strategy_versions.up.sql",
            "004_create_jobs.up.sql",
            "005_create_job_logs.up.sql",
            "006_create_performance_metrics.up.sql",
            "009_create_refresh_tokens.up.sql",
        ]:
            with open(os.path.join(_MIGRATIONS_DIR, migration)) as f:
                cur.execute(f.read())

        cur.execute(_PORTFOLIO_DDL)
        cur.execute(_MARKET_DATA_DDL)
        cur.close()
        conn.close()

        yield dsn


def _seed(dsn, strategy_code=None):
    """Insert user + strategy + version + job. Returns dict with IDs."""
    if strategy_code is None:
        strategy_code = VALID_STRATEGY
    user_id = str(uuid.uuid4())
    strategy_id = str(uuid.uuid4())
    version_id = str(uuid.uuid4())
    job_id = str(uuid.uuid4())
    s3_key = f"{user_id}/{strategy_id}/v1/main.py"

    conn = psycopg2.connect(dsn)
    with conn:
        with conn.cursor() as cur:
            cur.execute(
                "INSERT INTO users (id, email, password_hash) VALUES (%s, %s, %s)",
                (user_id, f"{job_id[:8]}@test.com", "x"),
            )
            cur.execute(
                "INSERT INTO strategies (id, user_id, name) VALUES (%s, %s, %s)",
                (strategy_id, user_id, "Test"),
            )
            cur.execute(
                "INSERT INTO strategy_versions (id, strategy_id, version_number, s3_key)"
                " VALUES (%s, %s, %s, %s)",
                (version_id, strategy_id, 1, s3_key),
            )
            cur.execute(
                "INSERT INTO jobs"
                " (id, user_id, strategy_version_id, type, status, symbols, resolution, start_date, end_date)"
                " VALUES (%s, %s, %s, 'backtest', 'queued', ARRAY['SPY'], '1d', '2024-01-02', '2024-01-05')",
                (job_id, user_id, version_id),
            )
    conn.close()
    return {
        "job_id": job_id,
        "user_id": user_id,
        "s3_key": s3_key,
        "strategy_code": strategy_code,
    }


def _get_job(dsn, job_id):
    conn = psycopg2.connect(dsn, cursor_factory=psycopg2.extras.RealDictCursor)
    with conn:
        with conn.cursor() as cur:
            cur.execute("SELECT status, error_message FROM jobs WHERE id=%s", (job_id,))
            return dict(cur.fetchone())


def _count(dsn, table, job_id):
    conn = psycopg2.connect(dsn)
    with conn:
        with conn.cursor() as cur:
            cur.execute(f"SELECT COUNT(*) FROM {table} WHERE job_id=%s", (job_id,))
            return cur.fetchone()[0]


def _make_lean_mock(tmp_dir):
    """Returns a side_effect function that writes the fixture JSON and returns the path."""

    def _mock(job_id, job_dir, timeout_seconds=7200):
        results_dir = os.path.join(job_dir, "Results")
        os.makedirs(results_dir, exist_ok=True)
        path = os.path.join(results_dir, "result.json")
        with open(path, "w") as f:
            json.dump(LEAN_RESULTS, f)
        return path

    return _mock


def _mock_requests_post():
    resp = MagicMock()
    resp.raise_for_status = MagicMock()
    resp.json.return_value = {"bars_ready": 3}
    return resp


def _run_task(dsn, job_id, s3_key, strategy_code, lean_side_effect):
    """Run the backtest task with moto S3 and mocked lean_runner."""
    import celery_worker

    # Patch DB and S3 config on the module
    celery_worker.DATABASE_URL = dsn
    celery_worker.S3_ENDPOINT = None
    celery_worker.S3_ACCESS_KEY = "test"
    celery_worker.S3_SECRET_KEY = "test"
    celery_worker.S3_BUCKET = "atp-strategies"
    celery_worker.S3_REGION = "us-east-1"

    with patch("celery_worker.run_lean_backtest", side_effect=lean_side_effect), \
         patch("celery_worker.requests.post", return_value=_mock_requests_post()):
        celery_worker.run_lean_backtest_task.apply(args=[job_id])


@mock_aws
def test_happy_path(pg_dsn, tmp_path):
    data = _seed(pg_dsn)
    job_id = data["job_id"]

    s3 = boto3.client("s3", region_name="us-east-1",
                      aws_access_key_id="test", aws_secret_access_key="test")
    s3.create_bucket(Bucket="atp-strategies")
    s3.put_object(Bucket="atp-strategies", Key=data["s3_key"],
                  Body=data["strategy_code"].encode())

    _run_task(pg_dsn, job_id, data["s3_key"], data["strategy_code"],
              _make_lean_mock(str(tmp_path)))

    job = _get_job(pg_dsn, job_id)
    assert job["status"] == "completed", f"expected completed, got {job['status']}: {job['error_message']}"

    pm_count = _count(pg_dsn, "performance_metrics", job_id)
    assert pm_count == 1, f"expected 1 performance_metrics row, got {pm_count}"

    port_count = _count(pg_dsn, "portfolio_metrics", job_id)
    assert port_count >= 1, f"expected at least 1 portfolio_metrics row, got {port_count}"

    # Verify sharpe_ratio was stored
    conn = psycopg2.connect(pg_dsn, cursor_factory=psycopg2.extras.RealDictCursor)
    with conn:
        with conn.cursor() as cur:
            cur.execute("SELECT sharpe_ratio FROM performance_metrics WHERE job_id=%s", (job_id,))
            row = dict(cur.fetchone())
    conn.close()
    assert row["sharpe_ratio"] is not None, "sharpe_ratio should be populated"


@mock_aws
def test_strategy_validation_failure(pg_dsn, tmp_path):
    data = _seed(pg_dsn, strategy_code=INVALID_STRATEGY)
    job_id = data["job_id"]

    s3 = boto3.client("s3", region_name="us-east-1",
                      aws_access_key_id="test", aws_secret_access_key="test")
    s3.create_bucket(Bucket="atp-strategies")
    s3.put_object(Bucket="atp-strategies", Key=data["s3_key"],
                  Body=INVALID_STRATEGY.encode())

    _run_task(pg_dsn, job_id, data["s3_key"], INVALID_STRATEGY,
              _make_lean_mock(str(tmp_path)))

    job = _get_job(pg_dsn, job_id)
    assert job["status"] == "failed"
    assert job["error_message"] is not None
    # Should mention the blocked import
    assert "os" in job["error_message"].lower() or "import" in job["error_message"].lower()


@mock_aws
def test_lean_timeout(pg_dsn, tmp_path):
    data = _seed(pg_dsn)
    job_id = data["job_id"]

    s3 = boto3.client("s3", region_name="us-east-1",
                      aws_access_key_id="test", aws_secret_access_key="test")
    s3.create_bucket(Bucket="atp-strategies")
    s3.put_object(Bucket="atp-strategies", Key=data["s3_key"],
                  Body=data["strategy_code"].encode())

    def timeout_mock(job_id, job_dir, timeout_seconds=7200):
        raise TimeoutError("LEAN backtest exceeded timeout")

    _run_task(pg_dsn, job_id, data["s3_key"], data["strategy_code"], timeout_mock)

    job = _get_job(pg_dsn, job_id)
    assert job["status"] == "failed"
    assert "timeout" in (job["error_message"] or "").lower()


@mock_aws
def test_lean_runtime_error(pg_dsn, tmp_path):
    data = _seed(pg_dsn)
    job_id = data["job_id"]

    s3 = boto3.client("s3", region_name="us-east-1",
                      aws_access_key_id="test", aws_secret_access_key="test")
    s3.create_bucket(Bucket="atp-strategies")
    s3.put_object(Bucket="atp-strategies", Key=data["s3_key"],
                  Body=data["strategy_code"].encode())

    def runtime_error_mock(job_id, job_dir, timeout_seconds=7200):
        raise RuntimeError("container exited 1")

    _run_task(pg_dsn, job_id, data["s3_key"], data["strategy_code"], runtime_error_mock)

    job = _get_job(pg_dsn, job_id)
    assert job["status"] == "failed"
    assert job["error_message"]  # non-empty
