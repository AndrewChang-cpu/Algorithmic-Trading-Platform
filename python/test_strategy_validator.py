import pytest
from strategy_validator import validate_strategy

VALID_STRATEGY = """
from AlgorithmImports import *

class MyStrategy(QCAlgorithm):
    def Initialize(self):
        self.SetStartDate(2020, 1, 1)
        self.AddEquity("SPY", Resolution.Daily)
    def OnData(self, data):
        pass
"""


def test_valid_strategy():
    result = validate_strategy(VALID_STRATEGY)
    assert result["valid"] is True
    assert result["class_name"] == "MyStrategy"


def test_blocked_import_os():
    code = "import os\n" + VALID_STRATEGY
    result = validate_strategy(code)
    assert result["valid"] is False
    assert "os" in result["violation"]


def test_blocked_import_subprocess():
    code = "import subprocess\n" + VALID_STRATEGY
    result = validate_strategy(code)
    assert result["valid"] is False
    assert "subprocess" in result["violation"]


def test_blocked_from_import():
    code = "from os import path\n" + VALID_STRATEGY
    result = validate_strategy(code)
    assert result["valid"] is False
    assert "os" in result["violation"]


def test_blocked_eval_call():
    code = VALID_STRATEGY + "\nx = eval('1+1')\n"
    result = validate_strategy(code)
    assert result["valid"] is False
    assert "eval" in result["violation"]


def test_no_qcalgorithm_subclass():
    code = """
class NotAStrategy:
    pass
"""
    result = validate_strategy(code)
    assert result["valid"] is False
    assert "QCAlgorithm" in result["violation"]


def test_multiple_classes_picks_qcalgorithm():
    code = """
class Helper:
    pass

class MyAlgo(QCAlgorithm):
    def Initialize(self):
        pass
"""
    result = validate_strategy(code)
    assert result["valid"] is True
    assert result["class_name"] == "MyAlgo"


def test_syntax_error():
    code = "def broken(:\n    pass\n"
    result = validate_strategy(code)
    assert result["valid"] is False
    assert "syntax error" in result["violation"].lower()


def test_importlib_blocked():
    result = validate_strategy("import importlib")
    assert result["valid"] is False
    assert "importlib" in result["violation"]


def test_ctypes_blocked():
    result = validate_strategy("import ctypes")
    assert result["valid"] is False


def test_builtins_blocked():
    result = validate_strategy("import builtins")
    assert result["valid"] is False


def test_attribute_import_blocked():
    result = validate_strategy("x = obj.__import__('os')")
    assert result["valid"] is False
    assert "__import__" in result["violation"]


def test_importlib_submodule_blocked():
    result = validate_strategy("from importlib.util import find_spec")
    assert result["valid"] is False


def test_blocks_import_pty():
    result = validate_strategy("import pty\nclass MyStrategy(QCAlgorithm):\n    pass\n")
    assert result["valid"] == False
    assert "pty" in result.get("violation", "")


def test_blocks_import_pickle():
    result = validate_strategy(
        "import pickle\nclass MyStrategy(QCAlgorithm):\n    pass\n"
    )
    assert result["valid"] == False
    assert "pickle" in result.get("violation", "")


def test_blocks_open_call():
    result = validate_strategy(
        'class MyStrategy(QCAlgorithm):\n    def foo(self):\n        open("secret.txt")\n'
    )
    assert result["valid"] == False
    assert "open" in result.get("violation", "")


def test_blocks_breakpoint_call():
    result = validate_strategy(
        "class MyStrategy(QCAlgorithm):\n    def foo(self):\n        breakpoint()\n"
    )
    assert result["valid"] == False
    assert "breakpoint" in result.get("violation", "")


def test_blocked_dunder_class():
    result = validate_strategy("().__class__.__subclasses__()")
    assert result["valid"] == False
    violation = result.get("violation", "")
    assert "__class__" in violation or "__subclasses__" in violation


def test_blocked_dunder_globals():
    result = validate_strategy("def f(): return f.__globals__")
    assert result["valid"] == False
    assert "__globals__" in result.get("violation", "")


def test_blocked_builtin_vars():
    result = validate_strategy("x = vars()")
    assert result["valid"] == False
    assert "vars" in result.get("violation", "")


def test_blocked_module_requests():
    result = validate_strategy("import requests")
    assert result["valid"] == False


def test_blocked_module_threading():
    result = validate_strategy("import threading")
    assert result["valid"] == False


@pytest.mark.parametrize("source,expected_valid,expected_class", [
    (
        "class Base(QCAlgorithm): pass\nclass Real(QCAlgorithm): pass",
        True,
        "Real",
    ),
    (
        "class MyAlgo(QCAlgorithm): pass",
        True,
        "MyAlgo",
    ),
])
def test_multiple_qc_classes(source, expected_valid, expected_class):
    result = validate_strategy(source)
    assert result["valid"] is expected_valid
    assert result["class_name"] == expected_class
