from celery import Celery
import backtrader as bt
from strategy import TestStrategy
import sys

######################
# This seems to be necessary to run Celery because backtrader.utils.flushfile isn't correct
# Remove if needed
class FlushFile:
    def __init__(self, f):
        self.f = f

    def write(self, x):
        self.f.write(x)
        self.f.flush()

    def flush(self):
        self.f.flush()

    def isatty(self):
        return False

sys.stdout = FlushFile(sys.stdout)
sys.stderr = FlushFile(sys.stderr)
######################

app = Celery('tasks', broker='redis://localhost:6379/0')

@app.task
def run_test_strategy(stake):
    # Configure and run the TestStrategy instance
    cerebro = bt.Cerebro()
    cerebro.addstrategy(TestStrategy, stake=stake)
    cerebro.run()
    return "Strategy executed with stake: {}".format(stake)

# Usage
# - curl -X POST http://localhost:8080/publish_task -H "Content-Type: application/json" -d '{"stake": 10}'