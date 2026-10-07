#!/usr/bin/env python3
"""
Runs the MI artifact tests: tests/test_*.py plus tests/<module>/test_*.py.

Module folders use hyphens (account-balance), which are not importable
package names, so test files are loaded by path instead of via
`unittest discover`. Exit code is non-zero if any test fails.
"""
import importlib.util
import sys
import unittest
from pathlib import Path

TESTS_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(TESTS_DIR))


def load_tests():
    suite = unittest.TestSuite()
    loader = unittest.TestLoader()
    for path in sorted(TESTS_DIR.glob("test_*.py")) + sorted(TESTS_DIR.glob("*/test_*.py")):
        name = f"mi_tests_{path.parent.name}_{path.stem}".replace("-", "_")
        spec = importlib.util.spec_from_file_location(name, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        suite.addTests(loader.loadTestsFromModule(module))
    return suite


if __name__ == "__main__":
    result = unittest.TextTestRunner(verbosity=2).run(load_tests())
    sys.exit(0 if result.wasSuccessful() else 1)
