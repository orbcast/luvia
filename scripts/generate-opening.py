#!/usr/bin/env python3
"""生成 Luvia 的启播动画（APNG）——点击播放后在播放器建立起播前播放的一段品牌动画。

动画风格参考用户提供的素材：蓝色球体从小放大到满尺寸，随后底部
淡入品牌字样。这里把品牌字样换成 Luvia，母版素材取自 assets/icons/mark.webp
（透明底蓝色球+镂空播放键），因此更换图标后重新生成本动画即可保持一致。

品牌字样由字形轮廓直接画成，不依赖本机安装的字体：不同机器的度量与
栅格化结果差别很大，用 ImageDraw.text 会让同一份脚本在不同平台产出
不同宽度的字样。需要 fontTools（`pip install fonttools`）。

输出两份完全相同的文件：
  - assets/brand/playintro/luvia-playintro.apng 动画母版（正式素材，体积不受限）
  - apps/desktop/internal/playintro/playintro.apng 应用内嵌副本（go:embed 使用）

用法（仓库根目录）：make brand，或 python3 -B scripts/generate-opening.py
"""

from __future__ import annotations

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

BALL = os.path.join(ROOT, "assets", "icons", "mark.webp")
MASTER_OUT = os.path.join(ROOT, "assets", "brand", "playintro", "luvia-playintro.apng")
EMBED_OUT = os.path.join(ROOT, "apps", "desktop", "internal", "playintro", "playintro.apng")

CANVAS = 512          # 画布边长，与开屏动画素材一致
FRAME_MS = 33         # 约 30fps
DURATION = 2.2        # 总时长（秒）
WORDMARK = "Luvia"
BALL_SIZE = 300       # 球体满尺寸边长
BALL_CENTER_Y = 215   # 球心纵坐标
WORDMARK_BASELINE = 415

# 字样的字形来源。以仓库内这一份为准：不同字体的字形宽高比差很多
# （Arial Bold 3.88、Segoe UI Bold 3.66、Ubuntu Bold 3.04、DejaVu 4.47），
# 按区台挑字体等于按区台挑字形，同一份脚本会产出不同的字样。
# 其余各项只是这个文件缺失时的退路。
GLYPH_CANDIDATES = [
    os.path.join(ROOT, "assets", "brand", "playintro", "wordmark.ttf"),
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
    os.path.join(os.environ.get("WINDIR", r"C:\Windows"), "Fonts", "arialbd.ttf"),
]

# 字样墨迹的高度。球体是 300 宽，字样按这个高度放出来约等于球的宽度
# （Arial Bold 的墨迹宽高比是 3.88，算出约 330），视觉重量才对得上；
# 再小就变成一行配不上球的小字。
WORDMARK_CAP = 85
# 轮廓先放大到 4 倍再缩回目标尺寸，边缘只有一格过渡；直接在目标尺寸上
# 填多边形会出锯齿。
WORDMARK_SUPERSAMPLE = 4

TOKEN = re.compile(r"[MLQHVZ]|-?\d+(?:\.\d+)?")
NUMBER = re.compile(r"^-?\d+(?:\.\d+)?$")


def load_glyph_source() -> tuple:
    """Return the font file and its outlines for WORDMARK."""
    try:
        from fontTools.pens.svgPathPen import SVGPathPen
        from fontTools.ttLib import TTFont
    except ImportError:
        sys.exit("generate-opening: 需要 fontTools，先 pip install fonttools")
    for path in GLYPH_CANDIDATES:
        if os.path.exists(path):
            break
    else:
        sys.exit("generate-opening: 找不到可用的字形字体：" + "、".join(GLYPH_CANDIDATES))
    font = TTFont(path)
    outlines = font.getGlyphSet()
    cmap = font.getBestCmap()
    metrics = font["hmtx"]
    rings: list = []
    x = 0
    for ch in WORDMARK:
        name = cmap[ord(ch)]
        pen = SVGPathPen(outlines, ntos=lambda value: "%.4f" % value)
        outlines[name].draw(pen)
        rings.append((rings_of(pen.getCommands()), x))
        x += metrics[name][0]
    xs = [px + offset for glyph, offset in rings for ring in glyph for px, _ in ring]
    ys = [py for glyph, _ in rings for ring in glyph for _, py in ring]
    return rings, (min(xs), min(ys), max(xs), max(ys))


def rings_of(commands: str) -> list:
    """Trace one glyph's SVG path into closed rings of points."""
    tokens = TOKEN.findall(commands)
    out: list = []
    ring: list = []
    current = (0.0, 0.0)
    i = 0
    while i < len(tokens):
        token = tokens[i]
        if token == "M":
            if len(ring) > 2:
                out.append(ring)
            current = (float(tokens[i + 1]), float(tokens[i + 2]))
            ring = [current]
            i += 3
        elif token == "L":
            current = (float(tokens[i + 1]), float(tokens[i + 2]))
            ring.append(current)
            i += 3
        elif token == "Q":
            control = (float(tokens[i + 1]), float(tokens[i + 2]))
            end = (float(tokens[i + 3]), float(tokens[i + 4]))
            for step in range(1, 9):
                u = step / 8
                ring.append((
                    (1 - u) ** 2 * current[0] + 2 * (1 - u) * u * control[0] + u * u * end[0],
                    (1 - u) ** 2 * current[1] + 2 * (1 - u) * u * control[1] + u * u * end[1],
                ))
            current = end
            i += 5
        elif token == "H":
            current = (float(tokens[i + 1]), current[1])
            ring.append(current)
            i += 2
        elif token == "V":
            current = (current[0], float(tokens[i + 1]))
            ring.append(current)
            i += 2
        elif token == "Z":
            if len(ring) > 2:
                out.append(ring)
            ring = []
            i += 1
        elif NUMBER.match(token):
            # SVGPathPen 省略重复命令字母，M 之后的顶点对仍是连线。
            current = (float(tokens[i]), float(tokens[i + 1]))
            ring.append(current)
            i += 2
        else:
            raise ValueError("generate-opening: 看不懂的字形路径 " + token)
    if len(ring) > 2:
        out.append(ring)
    return out


def wordmark_mask(rings: list, box: tuple, cap: int = WORDMARK_CAP, baseline: int = WORDMARK_BASELINE):
    """The wordmark's ink as a canvas-sized mask, centred and at cap height."""
    from PIL import Image, ImageDraw

    scale = cap * WORDMARK_SUPERSAMPLE / (box[3] - box[1])
    width = (box[2] - box[0]) * scale
    left = CANVAS * WORDMARK_SUPERSAMPLE / 2 - width / 2 - box[0] * scale
    big = Image.new("L", (CANVAS * WORDMARK_SUPERSAMPLE,) * 2, 0)
    draw = ImageDraw.Draw(big)
    for glyph, offset in rings:
        for ring in glyph:
            draw.polygon(
                [
                    (
                        left + (px + offset) * scale,
                        baseline * WORDMARK_SUPERSAMPLE - py * scale,
                    )
                    for px, py in ring
                ],
                fill=255,
            )
    return big.resize((CANVAS, CANVAS), Image.BOX)


def ease_out_back(t: float, s: float = 1.70158) -> float:
    t -= 1.0
    return 1.0 + t * t * ((s + 1.0) * t + s)


def ease_out(t: float) -> float:
    return 1.0 - (1.0 - t) ** 3


def build_frames() -> list:
    from PIL import Image

    if not os.path.exists(BALL):
        sys.exit(f"generate-opening: 缺少母版 {BALL}")
    ball = Image.open(BALL).convert("RGBA")
    rings, box = load_glyph_source()
    wordmark = wordmark_mask(rings, box)
    count = int(round(DURATION / (FRAME_MS / 1000.0)))
    frames: list = []
    for i in range(count):
        t = i * FRAME_MS / 1000.0
        canvas = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))

        # 球体：0~0.55s 从小放大（带回弹），同时前 0.28s 淡入。
        scale = 0.2 + max(0.0, ease_out_back(t / 0.55)) * 0.8 if t < 0.55 else 1.0
        alpha = 1.0 if t >= 0.28 else max(0.0, t / 0.28)
        size = int(BALL_SIZE * scale)
        if size > 0:
            sphere = ball.resize((size, size), Image.LANCZOS)
            if alpha < 1.0:
                sphere.putalpha(sphere.getchannel("A").point(lambda v: int(v * alpha)))
            canvas.alpha_composite(sphere, ((CANVAS - size) // 2, BALL_CENTER_Y - size // 2))

        # 字样：0.75s 起 0.6s 内淡入，并轻微上移。
        if t >= 0.75:
            progress = min((t - 0.75) / 0.6, 1.0)
            ink = int(255 * ease_out(progress))
            layer = Image.new("RGBA", (CANVAS, CANVAS), (255, 255, 255, 0))
            layer.putalpha(wordmark.point(lambda value: value * ink // 255))
            canvas.alpha_composite(layer, (0, int((1 - ease_out(progress)) * 10)))

        frames.append(canvas)
    return frames


def main() -> int:
    frames = build_frames()
    for out in (MASTER_OUT, EMBED_OUT):
        os.makedirs(os.path.dirname(out), exist_ok=True)
        frames[0].save(
            out,
            format="PNG",
            save_all=True,
            append_images=frames[1:],
            duration=FRAME_MS,
            loop=0,
        )
        print(f"generate-opening: {len(frames)} 帧 -> {os.path.relpath(out, ROOT)} ({os.path.getsize(out)} 字节)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
