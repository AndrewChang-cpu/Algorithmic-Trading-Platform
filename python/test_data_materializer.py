import os, zipfile, pytest
from datetime import datetime, timezone
from data_materializer import (
    materialize_lean_csv,
    _milliseconds_since_midnight as _to_ms,
)


def test_daily_bar_price_scaling(tmp_path):
    rows = [
        {
            "time": datetime(2024, 1, 2, tzinfo=timezone.utc),
            "open": 150.25,
            "high": 151.00,
            "low": 149.50,
            "close": 150.75,
            "volume": 1000000,
        }
    ]
    materialize_lean_csv(rows, str(tmp_path), "SPY", "1d")
    zip_path = tmp_path / "equity/usa/daily/spy/20240102_trade.zip"
    assert zip_path.exists()
    with zipfile.ZipFile(zip_path) as zf:
        content = zf.read("20240102_trade.csv").decode()
    lines = content.strip().split("\n")
    assert lines[0] == "Milliseconds,Open,High,Low,Close,Volume"
    vals = lines[1].split(",")
    assert vals[0] == "0"  # daily → milliseconds=0
    assert vals[1] == "1502500"  # 150.25 * 10000
    assert vals[2] == "1510000"  # 151.00 * 10000
    assert vals[3] == "1495000"  # 149.50 * 10000
    assert vals[4] == "1507500"  # 150.75 * 10000
    assert vals[5] == "1000000"


def test_minute_bar_milliseconds(tmp_path):
    rows = [
        {
            "time": datetime(2024, 1, 2, 9, 30, 0, tzinfo=timezone.utc),
            "open": 100.0,
            "high": 100.5,
            "low": 99.5,
            "close": 100.25,
            "volume": 5000,
        }
    ]
    materialize_lean_csv(rows, str(tmp_path), "SPY", "1m")
    zip_path = tmp_path / "equity/usa/minute/spy/20240102_trade.zip"
    assert zip_path.exists()
    with zipfile.ZipFile(zip_path) as zf:
        content = zf.read("20240102_trade.csv").decode()
    lines = content.strip().split("\n")
    vals = lines[1].split(",")
    assert vals[0] == "34200000"  # 9*3600000 + 30*60000 = 34200000 ms since midnight


def test_directory_structure(tmp_path):
    rows = [
        {
            "time": datetime(2024, 3, 15, tzinfo=timezone.utc),
            "open": 1.0,
            "high": 1.0,
            "low": 1.0,
            "close": 1.0,
            "volume": 100,
        }
    ]
    materialize_lean_csv(rows, str(tmp_path), "AAPL", "1h")
    expected_dir = tmp_path / "equity/usa/hour/aapl"
    assert expected_dir.is_dir()


def test_multiple_dates(tmp_path):
    rows = [
        {
            "time": datetime(2024, 1, 2, tzinfo=timezone.utc),
            "open": 1.0,
            "high": 1.0,
            "low": 1.0,
            "close": 1.0,
            "volume": 100,
        },
        {
            "time": datetime(2024, 1, 3, tzinfo=timezone.utc),
            "open": 2.0,
            "high": 2.0,
            "low": 2.0,
            "close": 2.0,
            "volume": 200,
        },
    ]
    materialize_lean_csv(rows, str(tmp_path), "SPY", "1d")
    assert (tmp_path / "equity/usa/daily/spy/20240102_trade.zip").exists()
    assert (tmp_path / "equity/usa/daily/spy/20240103_trade.zip").exists()


def test_to_ms_includes_microseconds():
    dt = datetime(2024, 1, 2, 9, 30, 0, 500000)  # 9:30:00.500
    assert _to_ms(dt, "1m") == 34200500  # 9*3600000 + 30*60000 + 500
