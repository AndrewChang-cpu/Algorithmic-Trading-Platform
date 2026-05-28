from AlgorithmImports import *


class TechMomentumRotation(QCAlgorithm):
    """
    Multi-stock momentum rotation strategy.

    Universe: 8 large-cap tech/growth equities.
    Selection: monthly rebalance picks the top 4 by 20-day momentum.
    Entry filter: RSI(14) must be between 40 and 70 (not overbought/oversold).
    Exit: EMA(10) crosses below EMA(30), or RSI > 75 (overbought exit).
    Position sizing: equal weight across held positions (25% each, max 4 slots).
    Risk management: 8% trailing stop per position.
    """

    UNIVERSE = ["AAPL", "MSFT", "GOOGL", "AMZN", "NVDA", "META", "TSLA", "AMD"]
    MAX_POSITIONS = 4
    REBALANCE_DAYS = 20

    def initialize(self):
        self.set_start_date(2022, 1, 1)
        self.set_end_date(2023, 12, 31)
        self.set_cash(100_000)
        self.set_benchmark("SPY")

        self._symbols = []
        self._indicators = {}
        self._trailing_stops = {}
        self._days_since_rebalance = 0

        for ticker in self.UNIVERSE:
            symbol = self.add_equity(ticker, Resolution.DAILY).symbol
            self._symbols.append(symbol)
            self._indicators[symbol] = {
                "ema_fast": self.ema(symbol, 10, Resolution.DAILY),
                "ema_slow": self.ema(symbol, 30, Resolution.DAILY),
                "rsi": self.rsi(symbol, 14, MovingAverageType.WILDERS, Resolution.DAILY),
                "momentum": self.momp(symbol, 20, Resolution.DAILY),
            }

        self.set_warm_up(35, Resolution.DAILY)

    def on_data(self, data: Slice):
        if self.is_warming_up:
            return

        self._check_trailing_stops(data)

        self._days_since_rebalance += 1
        if self._days_since_rebalance < self.REBALANCE_DAYS:
            return

        self._days_since_rebalance = 0
        self._rebalance(data)

    def _check_trailing_stops(self, data: Slice):
        for symbol, stop_price in list(self._trailing_stops.items()):
            if symbol not in data.bars:
                continue
            price = data.bars[symbol].close
            # Update trailing stop upward only
            new_stop = price * 0.92
            if new_stop > stop_price:
                self._trailing_stops[symbol] = new_stop
            # Trigger stop
            if price <= self._trailing_stops[symbol]:
                self.liquidate(symbol)
                del self._trailing_stops[symbol]
                self.log(f"Trailing stop triggered: {symbol.value} at {price:.2f}")

    def _rebalance(self, data: Slice):
        ranked = self._rank_by_momentum(data)
        target_symbols = ranked[:self.MAX_POSITIONS]
        target_weight = 1.0 / self.MAX_POSITIONS

        # Exit positions no longer in top-N or failing exit conditions
        for symbol in list(self.portfolio.keys()):
            if not self.portfolio[symbol].invested:
                continue
            if symbol not in target_symbols or self._should_exit(symbol, data):
                self.liquidate(symbol)
                self._trailing_stops.pop(symbol, None)
                self.log(f"Exiting {symbol.value}")

        # Enter new positions passing entry filter
        for symbol in target_symbols:
            if self.portfolio[symbol].invested:
                continue
            if not self._should_enter(symbol, data):
                continue
            self.set_holdings(symbol, target_weight)
            price = self.securities[symbol].price
            self._trailing_stops[symbol] = price * 0.92
            self.log(f"Entering {symbol.value} at {price:.2f}, stop at {self._trailing_stops[symbol]:.2f}")

    def _rank_by_momentum(self, data: Slice) -> list:
        scored = []
        for symbol in self._symbols:
            indics = self._indicators[symbol]
            if not all(i.is_ready for i in indics.values()):
                continue
            if symbol not in data.bars:
                continue
            scored.append((symbol, indics["momentum"].current.value))
        scored.sort(key=lambda x: x[1], reverse=True)
        return [s for s, _ in scored]

    def _should_enter(self, symbol, data: Slice) -> bool:
        if symbol not in data.bars:
            return False
        indics = self._indicators[symbol]
        if not all(i.is_ready for i in indics.values()):
            return False
        rsi_val = indics["rsi"].current.value
        ema_fast = indics["ema_fast"].current.value
        ema_slow = indics["ema_slow"].current.value
        # RSI in neutral zone and fast EMA above slow EMA (uptrend)
        return 40 <= rsi_val <= 70 and ema_fast > ema_slow

    def _should_exit(self, symbol, data: Slice) -> bool:
        if symbol not in data.bars:
            return False
        indics = self._indicators[symbol]
        if not all(i.is_ready for i in indics.values()):
            return False
        rsi_val = indics["rsi"].current.value
        ema_fast = indics["ema_fast"].current.value
        ema_slow = indics["ema_slow"].current.value
        # Overbought or EMA death cross
        return rsi_val > 75 or ema_fast < ema_slow

    def on_end_of_algorithm(self):
        invested = [s.value for s in self.portfolio.keys() if self.portfolio[s].invested]
        self.log(f"Final portfolio: {invested}")
        self.log(f"Total return: {(self.portfolio.total_portfolio_value / 100_000 - 1) * 100:.2f}%")
