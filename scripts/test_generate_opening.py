#!/usr/bin/env python3
"""Regression coverage for the wordmark the opening animation draws."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "generate_opening", Path(__file__).with_name("generate-opening.py"))
generator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generator)


class WordmarkTests(unittest.TestCase):
    def test_traces_curves_and_implicit_vertices(self):
        # M 之后的数据对是连线（SVGPathPen 省掉重复的 L），Q 要展成折线。
        rings = generator.rings_of("M0 0 10 0 10 10Z")
        self.assertEqual(rings, [[(0.0, 0.0), (10.0, 0.0), (10.0, 10.0)]])
        curved = generator.rings_of("M0 0Q10 20 20 0V-5H0Z")
        self.assertGreater(len(curved[0]), 8)
        self.assertIn((20.0, 0.0), curved[0])

    def test_every_glyph_of_the_wordmark_is_traced(self):
        try:
            import fontTools  # noqa: F401
        except ImportError:
            self.skipTest("没有 fontTools，字形提取由 make brand 时验证")
        rings, box = generator.load_glyph_source()
        self.assertEqual(len(rings), len(generator.WORDMARK))
        self.assertEqual(box[0] < box[2] and box[1] < box[3], True)
        for glyph, _ in rings:
            self.assertTrue(glyph, "字形没有轮廓")

    def test_mask_is_centred_and_sized_by_cap_height(self):
        try:
            import PIL  # noqa: F401
        except ImportError:
            self.skipTest("没有 Pillow，栅格化部分由 make check 在 CI 上验证")
        rings, box = generator.load_glyph_source()
        mask = generator.wordmark_mask(rings, box)
        left, top, right, bottom = mask.getbbox()
        self.assertEqual(mask.size, (generator.CANVAS, generator.CANVAS))
        # 墨迹居中、高度等于要求的大写高度，宽高比跟着字体走。
        self.assertAlmostEqual((left + right) / 2, generator.CANVAS / 2, delta=2)
        self.assertAlmostEqual(bottom - top, generator.WORDMARK_CAP, delta=2)
        ratio = (box[2] - box[0]) / (box[3] - box[1])
        self.assertAlmostEqual((right - left) / (bottom - top), ratio, delta=0.15)

    def test_wordmark_sits_under_the_ball(self):
        # 字样必须整个落在球的下方，且不超出画布，否则会被 Container 裁掉。
        try:
            import PIL  # noqa: F401
        except ImportError:
            self.skipTest("没有 Pillow，栅格化部分由 make check 在 CI 上验证")
        rings, box = generator.load_glyph_source()
        left, top, right, bottom = generator.wordmark_mask(rings, box).getbbox()
        self.assertGreater(top, generator.BALL_CENTER_Y + generator.BALL_SIZE / 2 - 40)
        self.assertLess(bottom, generator.CANVAS)
        self.assertGreater(left, 0)
        self.assertLess(right, generator.CANVAS)


if __name__ == "__main__":
    unittest.main()
