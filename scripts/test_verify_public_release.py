#!/usr/bin/env python3
"""Regression tests for the public-release safety scanner."""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SOURCE_SCANNER = Path(__file__).with_name("verify-public-release.py")


class VerifyPublicReleaseTests(unittest.TestCase):
    def test_ignores_marker_definitions_in_its_own_source(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "package"
            scanner = root / "scripts" / "verify-public-release.py"
            scanner.parent.mkdir(parents=True)
            shutil.copy2(SOURCE_SCANNER, scanner)

            result = subprocess.run(
                [sys.executable, str(scanner)], text=True, capture_output=True, check=False
            )

            self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_generated_cli_binary(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "package"
            scanner = root / "scripts" / "verify-public-release.py"
            scanner.parent.mkdir(parents=True)
            shutil.copy2(SOURCE_SCANNER, scanner)
            binary = root / "cli" / "trenda" / "trenda-pp-cli"
            binary.parent.mkdir(parents=True)
            binary.write_bytes(b"not source code")

            result = subprocess.run(
                [sys.executable, str(scanner)], text=True, capture_output=True, check=False
            )

            self.assertNotEqual(result.returncode, 0)
            self.assertIn("forbidden generated binary", result.stderr)


if __name__ == "__main__":
    unittest.main()
