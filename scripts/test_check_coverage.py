#!/usr/bin/env python3
"""Check that coverage counts source statements, including multi-package profiles."""
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


class CoverageTests(unittest.TestCase):
    def check_profile(self, content, minimum=80):
        with tempfile.TemporaryDirectory() as directory:
            profile = Path(directory) / "cover.out"
            profile.write_text(content, encoding="utf-8")
            return subprocess.run(
                [sys.executable, "-B", str(Path(__file__).with_name("check-coverage.py")),
                 "--profile", str(profile), "--min", str(minimum)],
                capture_output=True, text=True, check=False)

    def test_duplicate_blocks_do_not_inflate_denominator(self):
        result = self.check_profile("""mode: set
example/internal/shell.go:1.1,5.1 8 0
example/internal/shell.go:6.1,8.1 2 0
example/internal/shell.go:1.1,5.1 8 1
example/internal/shell.go:6.1,8.1 2 0
example/locales/catalog_gen.go:1.1,9.1 100 0
""")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("80.0% of 10 statements", result.stdout)

    def test_uncovered_entrypoint_still_counts(self):
        result = self.check_profile("""mode: set
example/internal/shell.go:1.1,5.1 8 1
example/main.go:1.1,5.1 3 0
""")
        self.assertEqual(result.returncode, 1)
        self.assertIn("72.7% of 11 statements", result.stdout)

    def test_empty_or_invalid_profiles_fail(self):
        for content in ("mode: set\n", "bad header\n"):
            with self.subTest(content=content):
                self.assertEqual(self.check_profile(content).returncode, 1)


if __name__ == "__main__":
    unittest.main()
