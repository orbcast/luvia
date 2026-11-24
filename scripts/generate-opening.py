#!/usr/bin/env python3
"""生成 Luvia 的启播动画（APNG）——点击播放后在播放器建立起播前播放的一段品牌动画。

动画只有那颗球：从小放大到满尺寸、前 0.28s 淡入，然后停住。母版素材取自
assets/icons/mark.webp（透明底蓝色球+镂空播放键），换图标后重新生成本动画
即可保持一致。

球下面原来还淡入一行 "Luvia" 字样（从字形轮廓直接画，避免各平台字体度量
不同）。它和球不在一个重量上——球是柔和的弥散渐变，字是纯白硬边的无衬线，
凑在一起像两张图拼起来的，所以整条字样链路连同 wordmark.ttf 一起删掉了。

输出两份完全相同的文件：
  - assets/brand/playintro/luvia-playintro.apng 动画母版（正式素材，体积不受限）
  - apps/desktop/internal/playintro/playintro.apng 应用内嵌副本（go:embed 使用）

用法（仓库根目录）：make brand，或 python3 -B scripts/generate-opening.py
"""

from __future__ import annotations

import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

BALL = os.path.join(ROOT, "assets", "icons", "mark.webp")
MASTER_OUT = os.path.join(ROOT, "assets", "brand", "playintro", "luvia-playintro.apng")
EMBED_OUT = os.path.join(ROOT, "apps", "desktop", "internal", "playintro", "playintro.apng")

CANVAS = 512          # 画布边长，与开屏动画素材一致
FRAME_MS = 33         # 约 30fps
DURATION = 2.2        # 总时长（秒）
BALL_SIZE = 300       # 球体满尺寸边长
BALL_CENTER_Y = 215   # 球心纵坐标


def ease_out_back(t: float, s: float = 1.70158) -> float:
    t -= 1.0
    return 1.0 + t * t * ((s + 1.0) * t + s)


def build_frames() -> list:
    from PIL import Image

    if not os.path.exists(BALL):
        sys.exit(f"generate-opening: 缺少母版 {BALL}")
    ball = Image.open(BALL).convert("RGBA")
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
