import os
import pytest
import psycopg2

_ALLOWED_TABLES = frozenset({"performance_metrics", "portfolio_metrics", "job_logs"})


def _count(dsn, table, job_id):
    if table not in _ALLOWED_TABLES:
        raise ValueError(f"Unknown table: {table!r}")
    conn = psycopg2.connect(dsn)
    try:
        with conn.cursor() as cur:
            cur.execute(f"SELECT COUNT(*) FROM {table} WHERE job_id = %s", (job_id,))
            return cur.fetchone()[0]
    finally:
        conn.close()


@pytest.fixture(scope="session", autouse=True)
def _set_celery_env():
    os.environ.setdefault("DATABASE_URL", "postgresql://test:test@localhost:5432/test")
    os.environ.setdefault("S3_ACCESS_KEY", "test-access-key")
    os.environ.setdefault("S3_SECRET_KEY", "test-secret-key")
    os.environ.setdefault("S3_BUCKET", "test-bucket")
    os.environ.setdefault("GO_DATA_URL", "http://localhost:8081")
    os.environ.setdefault("KAFKA_NODE_IP", "localhost")
    os.environ.setdefault("K8S_NAMESPACE", "default")
