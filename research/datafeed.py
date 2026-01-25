from __future__ import (absolute_import, division, print_function,
                        unicode_literals)

import datetime  # For datetime objects
import os.path  # To manage paths
import sys  # To find out the script name (in argv[0])

# Import the backtrader platform
import backtrader as bt

import datetime
import backtrader as bt
import backtrader.feeds as btfeeds


data = btfeeds.GenericCSVData(
    dataname='data.csv',
    
    # Date/time parameters
    datetime=1,  # timestamp column
    dtformat='%Y-%m-%d %H:%M:%S%z',  # matches your format with timezone
    
    # OHLCV columns
    open=2,
    high=3,
    low=4,
    close=5,
    volume=6,
    
    # Extra columns you don't need for backtrader
    openinterest=-1,  # not present in your data
    
    # Optional: date range filtering
    # fromdate=datetime.datetime(2016, 1, 1),
    # todate=datetime.datetime(2016, 12, 31),
)