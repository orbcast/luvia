# 共享资源

跨应用复用的品牌素材放在这里，`apps/` 下的每个应用都从这里取用，避免各平台各存一份。

## `icons/`

图标源文件。图形只画一次（矢量），各平台的位图从这里派生，尺寸在画布上约占 67%，四周留约 16% 内边距，不要在应用里再加 padding。

| 文件 | 说明 | 典型用途 |
|---|---|---|
| `mark.svg` | 矢量标志：透明底 + 蓝色圆球 + 镂空播放键，512×512 视图、1200×1200 画布 | 图标图形源、应用内展示（MyGo 的 `ui.Icon`/`ui.Image` 可直接吃 SVG） |
| `mark.webp` | 同一标志的 1024×1024 无损栅格版 | 无法解析 SVG 的场合（文档、商店页、其他平台） |
| `app-icon-dark.webp` | 黑色超椭圆（圆角方）底图 + 标志 | 默认应用图标：Windows、Linux、开发版 |
| `app-icon-light.webp` | 白色超椭圆底图 + 标志 | 备选配色 |
| `app-icon-dark-fullbleed.webp` | 黑色满版方图 + 标志 | 备选配色，给需要系统遮罩的平台 |

当前图标母版取自用户提供的蓝色球体图标集（黑/白超椭圆底、满版方图与透明标志）。`mark.svg` 仍是同一套设计的矢量标志（用户只给了位图，保留原矢量以保证 macOS Icon Composer 正常），其余位图母版由提供的 PNG 无损转为 WebP。

用 WebP 存栅格是为了体积（比同名 PNG 小约 40%）且无损，解码后与 PNG 逐像素一致。构建工具链只认 PNG / 平台格式，所以不要直接把这里的文件塞进应用：

```bash
make icons     # assets/icons -> 各应用需要的图标文件
```

`scripts/icons.sh` 把 `mark.svg` 复制给 macOS 的 Icon Composer 文档，并用 `dwebp`（或带 Pillow 的 `python3`）把 `app-icon-dark.webp` 转成 `apps/desktop/resources/icon.png`。生成物要提交进仓库，这样没有这些工具的机器也能直接构建；但不要手改它们，改母版再跑 `make icons`。

### 平台分工

- **macOS**：只给满版方图 + 矢量标志（`apps/desktop/AppIcon.icon/`，`fill` 黑色 + `mark.svg` 图层）。圆角遮罩、玻璃高光、投影全部由系统图标栈渲染——不要在图形里自己画超椭圆圆角或高光，否则会和系统遮罩叠成双层边。
- **Windows / Linux / 开发版**：这些平台没有系统遮罩，所以用烘焙好圆角的超椭圆底图（`app-icon-dark.webp` → `resources/icon.png`）。
- **浅色/深色**：只维护一套（黑底），不做深浅色两套；`make dev` 的开发版拿不到 Icon Composer 文档，两套外观会和正式包对不上。

换图标或调整内边距时只改母版，然后：

```bash
make icons
make build
```

母版的圆角与球体渐变都直接沿用原图，不要在合成时重画边缘或加高光；调整标志大小时只缩放标志本身，底图的圆角、抗锯齿边缘保持原样。

iOS / Android 等后续应用需要图标时，从这里取母版，在各自应用里生成平台格式（`AppIcon.appiconset`、`mipmap-*` 等），不要把母版复制过去。
