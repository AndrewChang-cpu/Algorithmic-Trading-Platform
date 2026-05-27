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
