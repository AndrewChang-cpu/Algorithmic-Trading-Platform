import json
from datetime import timezone
from results_parser import (
    parse_performance_metrics,
    parse_equity_curve,
    is_runtime_error,
)

# Matches structure of research/lean/lean-cli-test/My Project/backtests/.../1535553589-summary.json
SAMPLE_RESULTS = {
    "state": {"Status": "Completed", "RuntimeError": ""},
    "statistics": {"Sharpe Ratio": "8.854"},
    "totalPerformance": {
        "portfolioStatistics": {
            "startEquity": "100000",
            "endEquity": "101691.9198",
            "compoundingAnnualReturn": "2.7145",
            "drawdown": "0.022",
            "totalNetProfit": "0.0169",
            "sharpeRatio": "8.8543",
            "probabilisticSharpeRatio": "0.6761",
            "sortinoRatio": "0",
            "alpha": "-0.0049",
            "beta": "0.9961",
            "annualStandardDeviation": "0.2217",
            "annualVariance": "0.0491",
            "informationRatio": "-14.5651",
            "trackingError": "0.0009",
            "treynorRatio": "1.9704",
            "portfolioTurnover": "0.1993",
            "valueAtRisk99": "-0.028",
            "valueAtRisk95": "-0.019",
            "drawdownRecovery": "3",
            "winRate": "0",
            "lossRate": "0",
            "profitLossRatio": "0",
            "expectancy": "0",
        },
        "tradeStatistics": {
            "totalNumberOfTrades": 0,
            "numberOfWinningTrades": 0,
            "numberOfLosingTrades": 0,
            "totalFees": "0",
            "averageWin": "0",
            "averageLoss": "0",
            "largestProfit": "0",
            "largestLoss": "0",
            "maxConsecutiveWinningTrades": 0,
            "maxConsecutiveLosingTrades": 0,
            "averageMAE": "0",
            "averageMFE": "0",
        },
    },
    "charts": {
        "Strategy Equity": {
            "series": {
                "Equity": {
                    "values": [
                        [1381118400, 100000.0, 100000.0, 100000.0, 100000.0],
                        [1381377600, 99990.0, 100543.0, 98283.0, 98878.2174],
                    ]
                }
            }
        }
    },
}

RUNTIME_ERROR_RESULTS = {
    "state": {
        "Status": "RuntimeError",
        "RuntimeError": "Unable to locate symbol properties file",
    },
    "totalPerformance": {"portfolioStatistics": {}, "tradeStatistics": {}},
    "charts": {},
}


def test_not_runtime_error():
    ok, msg = is_runtime_error(SAMPLE_RESULTS)
    assert ok is False
    assert msg == ""


def test_is_runtime_error():
    ok, msg = is_runtime_error(RUNTIME_ERROR_RESULTS)
    assert ok is True
    assert "symbol properties" in msg


def test_sharpe_ratio():
    metrics = parse_performance_metrics(SAMPLE_RESULTS)
    assert abs(metrics["sharpe_ratio"] - 8.8543) < 0.001


def test_start_end_equity():
    metrics = parse_performance_metrics(SAMPLE_RESULTS)
    assert metrics["start_equity"] == 100000.0
    assert abs(metrics["end_equity"] - 101691.9198) < 0.01


def test_equity_curve_length():
    points = parse_equity_curve(SAMPLE_RESULTS)
    assert len(points) == 2


def test_equity_curve_first_point():
    points = parse_equity_curve(SAMPLE_RESULTS)
    assert points[0]["open"] == 100000.0
    assert points[0]["close"] == 100000.0
    assert points[0]["time"].tzinfo == timezone.utc


def test_empty_charts():
    points = parse_equity_curve(RUNTIME_ERROR_RESULTS)
    assert points == []
