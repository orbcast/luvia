#!/usr/bin/env python3
"""Validate upstream SVG handling without weakening authored-text hygiene."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "check_rules", Path(__file__).with_name("check-rules.py"))
rules = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rules)


class TextHygieneTests(unittest.TestCase):
    def check_file(self, path, content):
        with tempfile.TemporaryDirectory() as directory:
            file = Path(directory) / path
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_bytes(content)
            with patch.object(rules, "ROOT", directory):
                return rules.check_text_hygiene([path])

    def test_upstream_svg_can_preserve_absent_final_newline(self):
        self.assertEqual(self.check_file(
            "assets/ui-icons/solar/solar--home-linear.svg", b"<svg/>"), [])

    def test_other_files_still_require_final_newline(self):
        for path in ("README.md", "assets/icons/mark.svg",
                     "assets/ui-icons/solar/local.svg", "apps/desktop/main.go"):
            with self.subTest(path=path):
                self.assertTrue(any("缺少结尾换行" in item
                                    for item in self.check_file(path, b"text")))

    def test_upstream_svg_still_rejects_crlf_and_trailing_spaces(self):
        result = self.check_file("assets/ui-icons/solar/solar--home-linear.svg",
                                 b"<svg>\r\n<g/> \n</svg>")
        self.assertTrue(any("CRLF" in item for item in result))
        self.assertTrue(any("多余空白" in item for item in result))


if __name__ == "__main__":
    unittest.main()
