#!/usr/bin/env python3
"""生成 Luvia 的启播动画（APNG）——点击播放后在播放器建立起播前播放的一段品牌动画。

动画风格参考用户提供的素材：蓝色球体从小放大到满尺寸，随后底部
淡入品牌字样。这里把品牌字样换成 Luvia，母版素材取自 assets/icons/mark.webp
（透明底蓝色球+镂空播放键），因此更换图标后重新生成本动画即可保持一致。

输出两份完全相同的文件：
  - assets/brand/playintro/luvia-playintro.apng 动画母版（正式素材，体积不受限）
  - apps/desktop/internal/playintro/playintro.apng 应用内嵌副本（go:embed 使用）

用法（仓库根目录）：make brand，或 python3 -B scripts/generate-opening.py
"""

from __future__ import annotations

import os
import sys

from PIL import Image, ImageDraw, ImageFont

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

FONT_CANDIDATES = [
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
    "/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf",
]


def load_font(size: int) -> ImageFont.FreeTypeFont:
    for path in FONT_CANDIDATES:
        if os.path.exists(path):
            return ImageFont.truetype(path, size)
    return ImageFont.load_default()


def ease_out_back(t: float, s: float = 1.70158) -> float:
    t -= 1.0
    return 1.0 + t * t * ((s + 1.0) * t + s)


def ease_out(t: float) -> float:
    return 1.0 - (1.0 - t) ** 3


def build_frames() -> list[Image.Image]:
    if not os.path.exists(BALL):
        sys.exit(f"generate-opening: 缺少母版 {BALL}")
    ball = Image.open(BALL).convert("RGBA")
    font = load_font(66)
    count = int(round(DURATION / (FRAME_MS / 1000.0)))
    frames: list[Image.Image] = []
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
            draw = ImageDraw.Draw(canvas)
            box = draw.textbbox((0, 0), WORDMARK, font=font)
            x = (CANVAS - (box[2] - box[0])) // 2 - box[0]
            y = WORDMARK_BASELINE - box[1] + int((1 - ease_out(progress)) * 10)
            draw.text((x, y), WORDMARK, font=font, fill=(255, 255, 255, ink))

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
