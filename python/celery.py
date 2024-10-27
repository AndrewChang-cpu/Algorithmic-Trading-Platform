from celery import Celery
import backtrader as bt
from strategy import TestStrategy

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
