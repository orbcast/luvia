# 启播动画

`luvia-playintro.apng` 是点击播放、播放器建立起播前播放的一段品牌动画：深色底上蓝色球体从小放大到满尺寸，随后底部淡入“Luvia”字样。它不是应用启动的开屏动画。

## 来源与风格

- 风格参考用户提供的素材（球体放大 + 品牌字样淡入）；品牌字样为 Luvia，与当前应用品牌一致。
- 球体取自 [assets/icons/mark.webp](../../icons/README.md)（透明底蓝色球 + 镂空播放键），因此与桌面应用图标同源。

## 生成

母版由 [scripts/generate-opening.py](../../../scripts/generate-opening.py) 生成，输出两份完全相同的文件：

- `assets/brand/playintro/luvia-playintro.apng` —— 动画母版；
- `apps/desktop/internal/playintro/playintro.apng` —— 桌面应用 `go:embed` 的内嵌副本。

品牌字样的字形来自 `wordmark.ttf`（Arial Bold 的子集字体，随素材一起交付），脚本直接把它描成矢量多边形再栅格化，不调用本机字体。这样 Linux、macOS、Windows 上跑出来的字样一样：换字体等于换字形比例，Arial Bold 的墨迹宽高比是 3.88，Segoe UI Bold 3.66，Ubuntu Bold 3.04，DejaVu 4.47，按平台挑字体就会得到四行不同的字。字样墨迹高度取 `WORDMARK_CAP`（85px），画出约 330 宽，与 300 宽的球体视觉重量相匹配；再小就成了一行配不上球体的小字。

生成依赖 Pillow 与 fontTools（`pip install pillow fonttools`）。

改动动画只改生成脚本或图标母版，再重新生成，不手改生成物：

```sh
make brand
```

脚本的回归在 [scripts/test_generate_opening.py](../../../scripts/test_generate_opening.py)，由 `make test` 一起跑。

APNG 为透明底，桌面应用在点击播放时把它覆盖在窗口上（`design.PlayIntroBackground` 深色底）用 `ui.Video` 播放一次，放完再回到界面；`internal/playintro` 的自检会解码内嵌动画，确认它不是空文件。
