import ast

BLOCKED_MODULES = {"os", "subprocess", "socket", "sys", "shutil", "pathlib"}
BLOCKED_BUILTINS = {"eval", "exec", "__import__", "compile"}


def validate_strategy(source_code: str) -> dict:
    try:
        tree = ast.parse(source_code)
    except SyntaxError as e:
        return {"valid": False, "violation": f"syntax error: {e}"}

    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                for blocked in BLOCKED_MODULES:
                    if alias.name == blocked or alias.name.startswith(blocked + "."):
                        return {"valid": False, "violation": f"import {blocked} detected on line {node.lineno}"}

        elif isinstance(node, ast.ImportFrom):
            if node.module:
                for blocked in BLOCKED_MODULES:
                    if node.module == blocked or node.module.startswith(blocked + "."):
                        return {"valid": False, "violation": f"import {blocked} detected on line {node.lineno}"}

        elif isinstance(node, ast.Call):
            if isinstance(node.func, ast.Name) and node.func.id in BLOCKED_BUILTINS:
                name = node.func.id
                return {"valid": False, "violation": f"import {name} detected on line {node.lineno}"}

    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef):
            for base in node.bases:
                if isinstance(base, ast.Name) and base.id == "QCAlgorithm":
                    return {"valid": True, "class_name": node.name}
                if isinstance(base, ast.Attribute) and base.attr == "QCAlgorithm":
                    return {"valid": True, "class_name": node.name}

    return {"valid": False, "violation": "no QCAlgorithm subclass found"}
