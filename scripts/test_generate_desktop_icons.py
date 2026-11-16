#!/usr/bin/env python3
"""Regression coverage for deterministic, verbatim SVG embedding."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "generate_icons", Path(__file__).with_name("generate-desktop-icons.py"))
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


class IconGenerationTests(unittest.TestCase):
    def test_embeds_every_master_without_modification(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)
            for name in generator.ICONS:
                (source / f"solar--{name}-linear.svg").write_text(
                    f'<svg data-name="{name}" color="currentColor">\n</svg>\n',
                    encoding="utf-8")
            rendered = generator.render(source)
            self.assertEqual(rendered, generator.render(source))
            import json
            literals = rendered.split("var solarSources = [...]string{\n", 1)[1]
            values = [json.loads(line.strip().removesuffix(","))
                      for line in literals.splitlines() if line.startswith("\t")]
            self.assertEqual(len(values), 5)
            for name, value in zip(generator.ICONS, values):
                self.assertEqual(value, (source / f"solar--{name}-linear.svg").read_text())

    def test_missing_master_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(FileNotFoundError):
                generator.render(Path(directory))


if __name__ == "__main__":
    unittest.main()
