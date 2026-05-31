import ast

# Security note: The AST scan is a first-line UX check, not a security boundary.
# Docker container isolation (--network none / lean-live-net, --cap-drop ALL) is enforced separately.
BLOCKED_MODULES = {
    "os",
    "subprocess",
    "socket",
    "sys",
    "shutil",
    "pathlib",
    "importlib",
    "importlib.util",
    "importlib.machinery",
    "ctypes",
    "builtins",
    "pickle",
    "marshal",
    "pty",
    "urllib",
    "requests",
    "urllib3",
    "http",
    "http.client",
    "ftplib",
    "smtplib",
    "threading",
    "multiprocessing",
    "concurrent",
    "asyncio",
}
BLOCKED_BUILTINS = {
    "eval",
    "exec",
    "__import__",
    "compile",
    "open",
    "breakpoint",
    "getattr",
    "setattr",
    "delattr",
    "vars",
    "globals",
    "locals",
    "dir",
}
BLOCKED_ATTRS = {
    "__import__",
    "__builtins__",
    "__loader__",
    "__class__",
    "__subclasses__",
    "__globals__",
    "__dict__",
    "__mro__",
}


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
                        return {
                            "valid": False,
                            "violation": f"import {blocked} detected on line {node.lineno}",
                        }

        elif isinstance(node, ast.ImportFrom):
            if node.module:
                for blocked in BLOCKED_MODULES:
                    if node.module == blocked or node.module.startswith(blocked + "."):
                        return {
                            "valid": False,
                            "violation": f"import {blocked} detected on line {node.lineno}",
                        }

        elif isinstance(node, ast.Call):
            if isinstance(node.func, ast.Name) and node.func.id in BLOCKED_BUILTINS:
                name = node.func.id
                return {
                    "valid": False,
                    "violation": f"use of blocked builtin '{name}' on line {node.lineno}",
                }

        if isinstance(node, ast.Attribute) and node.attr in BLOCKED_ATTRS:
            return {
                "valid": False,
                "violation": f"use of blocked attribute '{node.attr}' on line {node.lineno}",
            }

    qc_classes = []
    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef):
            for base in node.bases:
                if (isinstance(base, ast.Name) and base.id == "QCAlgorithm") or \
                   (isinstance(base, ast.Attribute) and base.attr == "QCAlgorithm"):
                    qc_classes.append(node.name)

    if not qc_classes:
        return {"valid": False, "violation": "no QCAlgorithm subclass found"}
    return {"valid": True, "class_name": qc_classes[-1]}
