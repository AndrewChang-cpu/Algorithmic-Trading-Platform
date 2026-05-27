from datetime import datetime, timezone
from typing import Any


def is_runtime_error(results: dict) -> tuple[bool, str]:
    """Returns (True, error_message) if LEAN reported RuntimeError, else (False, '')."""
    state = results.get("state", {})
    if state.get("Status") == "RuntimeError":
        return True, state.get("RuntimeError", "Unknown error")
    return False, ""


def parse_performance_metrics(results: dict) -> dict:
    """
    Extract structured numeric metrics from totalPerformance.
    Returns a dict mapping to performance_metrics table column names.
    Prefer totalPerformance fields (numeric) over the string-formatted 'statistics' dict.
    All values are float or None if missing/unparseable.
    """
    ps = results.get("totalPerformance", {}).get("portfolioStatistics", {})
    ts = results.get("totalPerformance", {}).get("tradeStatistics", {})

    def f(d, key):
        v = d.get(key)
        if v is None:
            return None
        try:
            return float(v)
        except:
            return None

    def i(d, key):
        v = d.get(key)
        if v is None:
            return None
        try:
            return int(float(v))
        except:
            return None

    return {
        "total_return_pct": f(ps, "totalNetProfit"),
        "compounding_annual_return": f(ps, "compoundingAnnualReturn"),
        "alpha": f(ps, "alpha"),
        "beta": f(ps, "beta"),
        "sharpe_ratio": f(ps, "sharpeRatio"),
        "sortino_ratio": f(ps, "sortinoRatio"),
        "max_drawdown_pct": f(ps, "drawdown"),
        "drawdown_recovery_days": i(ps, "drawdownRecovery"),
        "volatility_annual": f(ps, "annualStandardDeviation"),
        "annual_variance": f(ps, "annualVariance"),
        "information_ratio": f(ps, "informationRatio"),
        "tracking_error": f(ps, "trackingError"),
        "treynor_ratio": f(ps, "treynorRatio"),
        "probabilistic_sharpe_ratio": f(ps, "probabilisticSharpeRatio"),
        "value_at_risk_99": f(ps, "valueAtRisk99"),
        "value_at_risk_95": f(ps, "valueAtRisk95"),
        "win_rate_pct": f(ps, "winRate"),
        "loss_rate_pct": f(ps, "lossRate"),
        "profit_loss_ratio": f(ps, "profitLossRatio"),
        "expectancy": f(ps, "expectancy"),
        "start_equity": f(ps, "startEquity"),
        "end_equity": f(ps, "endEquity"),
        "portfolio_turnover": f(ps, "portfolioTurnover"),
        "total_trades": i(ts, "totalNumberOfTrades"),
        "winning_trades": i(ts, "numberOfWinningTrades"),
        "losing_trades": i(ts, "numberOfLosingTrades"),
        "total_fees": f(ts, "totalFees"),
        "avg_win_pct": f(ts, "averageWin"),
        "avg_loss_pct": f(ts, "averageLoss"),
        "largest_win": f(ts, "largestProfit"),
        "largest_loss": f(ts, "largestLoss"),
        "max_consecutive_wins": i(ts, "maxConsecutiveWinningTrades"),
        "max_consecutive_losses": i(ts, "maxConsecutiveLosingTrades"),
        "avg_mae": f(ts, "averageMAE"),
        "avg_mfe": f(ts, "averageMFE"),
    }


def parse_equity_curve(results: dict) -> list[dict]:
    """
    Extract equity curve from charts["Strategy Equity"].series["Equity"].values.
    Returns list of {"time": datetime, "open": float, "high": float, "low": float, "close": float}.
    Each value element is [unix_timestamp_seconds, open, high, low, close].
    """
    try:
        values = (results["charts"]["Strategy Equity"]
                        ["series"]["Equity"]["values"])
    except (KeyError, TypeError):
        return []

    points = []
    for v in values:
        if len(v) < 5:
            continue
        ts, o, h, l, c = v[0], v[1], v[2], v[3], v[4]
        points.append({
            "time": datetime.fromtimestamp(ts, tz=timezone.utc),
            "open": float(o),
            "high": float(h),
            "low": float(l),
            "close": float(c),
        })
    return points
