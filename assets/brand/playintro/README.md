# 启播动画

`luvia-playintro.apng` 是点击播放、播放器建立起播前播放的一段品牌动画：深色底上蓝色球体从小放大到满尺寸，然后停住。它不是应用启动的开屏动画。

## 来源与风格

- 风格参考用户提供的素材（球体放大）；品牌字样曾淡入在球下方，因与球体视觉重量不匹配已去掉。
- 球体取自 [assets/icons/mark.webp](../../icons/README.md)（透明底蓝色球 + 镂空播放键），因此与桌面应用图标同源。

## 生成

母版由 [scripts/generate-opening.py](../../../scripts/generate-opening.py) 生成，输出两份完全相同的文件：

- `assets/brand/playintro/luvia-playintro.apng` —— 动画母版；
- `apps/desktop/internal/playintro/playintro.apng` —— 桌面应用 `go:embed` 的内嵌副本。

生成依赖 Pillow（`pip install pillow`）。

白实心无衬线的字和柔和的弥散渐变球不在一个重量上，凑在一起像两张图拼出来的，所以整个字样链路（字形提取、`wordmark.ttf`、相关用例）一起删掉了。要再加回品牌字，得先解决这个重量问题，不是把字挪进脚本就行。

改动动画只改生成脚本或图标母版，再重新生成，不手改生成物：

```sh
make brand
```

APNG 为透明底，桌面应用在点击播放时把它覆盖在窗口上（`design.PlayIntroBackground` 深色底）用 `ui.Video` 播放一次，放完再回到界面；`internal/playintro` 的自检会解码内嵌动画，确认它不是空文件。
