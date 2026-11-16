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
            for name, style in generator.ICONS:
                (source / f"solar--{name}-{style}.svg").write_text(
                    f'<svg data-name="{name}" data-style="{style}" color="currentColor">\n</svg>\n',
                    encoding="utf-8")
            rendered = generator.render(source)
            self.assertEqual(rendered, generator.render(source))
            literal = rendered.split("var Sources = map[string]string{\n", 1)[1]
            rows = [line.strip() for line in literal.splitlines() if line.startswith("\t")]
            self.assertEqual(len(rows), len(generator.ICONS))
            for (name, style), row in zip(generator.ICONS, rows):
                self.assertTrue(row.startswith(f'"{name}-{style}": '), row)
                embedded = row.split(": ", 1)[1].removesuffix(",")
                self.assertEqual(embedded, __import__("json").dumps(
                    (source / f"solar--{name}-{style}.svg").read_text(encoding="utf-8")))

    def test_missing_master_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(FileNotFoundError):
                generator.render(Path(directory))


if __name__ == "__main__":
    unittest.main()
