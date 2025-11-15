import backtrader as bt
import inspect

class DynamicStrategy(bt.Strategy):
    """Base strategy class where we dynamically inject user-defined functions."""
    pass

def validate_function(func_code, expected_name, expected_params):
    """ Validates a user-defined function before injecting it into the strategy. """
    exec_namespace = {}

    try:
        # Execute the function code safely
        exec(func_code, globals(), exec_namespace)

        # Extract function from executed namespace
        user_func = exec_namespace.get(expected_name)
        if not callable(user_func):
            raise ValueError(f"Provided code does not define a valid function '{expected_name}'.")

        # Validate function signature
        func_sig = inspect.signature(user_func)
        func_params = list(func_sig.parameters.keys())

        if func_params != expected_params:
            raise ValueError(f"Function '{expected_name}' must have parameters {expected_params}, but got {func_params}.")

        return user_func  # Return validated function

    except Exception as e:
        raise ValueError(f"Error in function '{expected_name}': {e}")

def run_user_strategy(init_code, notify_data_code, notify_order_code, next_code):
    """Dynamically adds user-defined methods to a Backtrader strategy with safety checks."""
    
    # Validate and inject user functions
    function_definitions = {
        "__init__": (init_code, ["self"]),
        "notify_data": (notify_data_code, ["self", "data", "status", "args", "kwargs"]),
        "notify_order": (notify_order_code, ["self", "order"]),
        "next": (next_code, ["self"])
    }

    for func_name, (func_code, expected_params) in function_definitions.items():
        user_func = validate_function(func_code, func_name, expected_params)
        setattr(DynamicStrategy, func_name, user_func)

    # Set up Backtrader with the dynamically created strategy
    cerebro = bt.Cerebro()
    cerebro.addstrategy(DynamicStrategy)  # Use the dynamically modified strategy

    # Create a simple data feed (could be replaced with real data)
    data = bt.feeds.GenericCSVData(
        dataname="sample.csv", dtformat="%Y-%m-%d", timeframe=bt.TimeFrame.Days
    )
    cerebro.adddata(data)

    # Run Backtrader
    cerebro.run()
    cerebro.plot()

# Example user-defined function strings
init_code = """
def __init__(self):
    print("Strategy Initialized")
"""

notify_data_code = """
def notify_data(self, data, status, *args, **kwargs):
    print(f"Data status changed: {status}")
"""

notify_order_code = """
def notify_order(self, order):
    print(f"Order Notification: {order.status}")
"""

next_code = """
def next(self):
    print(f"Next called, closing price: {self.datas[0].close[0]}")
"""

# Run the strategy with user-defined functions
run_user_strategy(init_code, notify_data_code, notify_order_code, next_code)
