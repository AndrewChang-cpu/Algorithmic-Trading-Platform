import os
import zipfile
from datetime import datetime, date


RESOLUTION_MAP = {
    "1d": "daily",
    "1h": "hour",
    "1m": "minute",
}


def _parse_time(t) -> datetime:
    if isinstance(t, datetime):
        return t
    # Handle ISO string, strip timezone suffix for fromisoformat compat
    s = str(t)
    # Remove timezone info if present
    for suffix in ("Z", "+00:00"):
        if s.endswith(suffix):
            s = s[: -len(suffix)]
    # Handle +HH:MM style tz offsets
    if "+" in s[10:]:
        s = s[: s.index("+", 10)]
    return datetime.fromisoformat(s)


def _milliseconds_since_midnight(dt: datetime, resolution: str) -> int:
    if resolution == "1d":
        return 0
    return (dt.hour * 3600 + dt.minute * 60 + dt.second) * 1000


def _scale_price(price) -> int:
    return int(round(float(price) * 10000))


def materialize_lean_csv(rows: list[dict], output_dir: str, symbol: str, resolution: str):
    lean_resolution = RESOLUTION_MAP[resolution]
    symbol_lower = symbol.lower()

    # Group rows by date
    by_date: dict[date, list[dict]] = {}
    for row in rows:
        dt = _parse_time(row["time"])
        d = dt.date()
        by_date.setdefault(d, []).append((dt, row))

    out_dir = os.path.join(output_dir, "equity", "usa", lean_resolution, symbol_lower)
    os.makedirs(out_dir, exist_ok=True)

    for d, date_rows in sorted(by_date.items()):
        date_str = d.strftime("%Y%m%d")

        lines = ["Milliseconds,Open,High,Low,Close,Volume"]
        for dt, row in date_rows:
            ms = _milliseconds_since_midnight(dt, resolution)
            open_  = _scale_price(row["open"])
            high   = _scale_price(row["high"])
            low    = _scale_price(row["low"])
            close  = _scale_price(row["close"])
            volume = int(row["volume"])
            lines.append(f"{ms},{open_},{high},{low},{close},{volume}")

        csv_content = "\n".join(lines)
        csv_filename = f"{date_str}_trade.csv"
        zip_path = os.path.join(out_dir, f"{date_str}_trade.zip")

        with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
            zf.writestr(csv_filename, csv_content)


def materialize_all(rows_by_symbol: dict[str, list[dict]], output_dir: str, resolution: str):
    for symbol, rows in rows_by_symbol.items():
        materialize_lean_csv(rows, output_dir, symbol, resolution)
