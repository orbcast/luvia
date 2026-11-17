# 开屏动画

Luvia 启动时的开屏动画母版 `luvia-opening.apng`：深色底上，蓝色球体从小放大到满尺寸，随后底部淡入“Luvia”字样，然后进入主窗口。

## 来源与风格

- 风格参考用户提供的 AssTV 开屏动画（球体放大 + 品牌字样淡入）；品牌字样替换为 Luvia，与当前应用品牌一致。
- 球体取自 [assets/icons/mark.webp](../../icons/README.md)（透明底蓝色球 + 镂空播放键），因此与桌面应用图标同源。

## 生成

母版由 [scripts/generate-opening.py](../../../scripts/generate-opening.py) 生成，输出两份完全相同的文件：

- `assets/brand/opening/luvia-opening.apng` —— 动画母版；
- `apps/desktop/internal/splash/opening.apng` —— 桌面应用 `go:embed` 的内嵌副本。

改动动画只改生成脚本或图标母版，再重新生成，不手改生成物：

```sh
make brand
```

APNG 为透明底，桌面应用在深色窗口（`design.SplashBackground`）上用 `ui.Video` 播放一次后进入主窗口；`internal/splash` 的自检会解码内嵌动画，确认它不是空文件。
