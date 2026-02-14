from AlgorithmImports import *

class IsolatedAlgorithm(QCAlgorithm):
    def Initialize(self):
        self.SetStartDate(2021, 1, 1)
        self.SetEndDate(2021, 1, 10)
        self.SetCash(100000)
        
        # Adding equity: the engine will look in /data/equity/usa/daily/spy.zip
        self.spy = self.AddEquity("SPY", Resolution.Daily).Symbol

    def OnData(self, data):
        if not self.Portfolio.Invested:
            self.SetHoldings(self.spy, 1.0)
            self.Log(f"Bought SPY at {self.Time}")